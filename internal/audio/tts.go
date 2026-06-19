package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/provider/tts"
)

// TTSChunk 是一个文本-音频配对单元，对应一句话。
type TTSChunk struct {
	Text  string
	Audio []byte
}

// TTSConfig 配置 TTSProcessor。
type TTSConfig struct {
	// Provider 是 TTS 后端。必填。
	Provider tts.Provider
	// MaxConcurrent 是并发合成的最大句数，默认 2。
	// 合成完的结果由 orderer 按原始顺序排列后送入播放队列。
	MaxConcurrent int
	// QueueSize 是待合成句子队列的大小，默认 100。
	QueueSize int
	// MaxRunes 是单句最大字符数（超过则强制切句），默认 100。
	MaxRunes int
}

// DefaultTTSConfig 返回合理的默认配置，Provider 必须由调用方设置。
func DefaultTTSConfig() *TTSConfig {
	return &TTSConfig{
		MaxConcurrent: 2,
		QueueSize:     100,
		MaxRunes:      100,
	}
}

// TTSProcessor 接收 LLM 流式文本，内部分句后并发合成音频，
// 以句为单位通过 OnChunk 按原始顺序回调文本+音频配对。
//
// 流水线：
//
//	sentenceCh → [worker×MaxConcurrent] → resultCh → [orderer] → playbackCh → [playback]
//
// 合成并发数与播放独立控制，playbackCh 容量=1，合成最多比播放提前 1 句。
type TTSProcessor interface {
	Write(text string, opts tts.SynthesisOptions) error
	Flush(opts tts.SynthesisOptions) error
	OnChunk(func(TTSChunk))
	Interrupt() error
	Start(ctx context.Context) error
	Stop() error
}

// NewTTSProcessor 创建 TTSProcessor。cfg.Provider 不能为 nil。
func NewTTSProcessor(cfg *TTSConfig) (TTSProcessor, error) {
	if cfg == nil {
		cfg = DefaultTTSConfig()
	}
	if cfg.Provider == nil {
		return nil, errorf("TTSProcessor: Provider is required")
	}
	maxC := cfg.MaxConcurrent
	if maxC <= 0 {
		maxC = 2
	}
	qs := cfg.QueueSize
	if qs <= 0 {
		qs = 100
	}
	mr := cfg.MaxRunes
	if mr <= 0 {
		mr = 100
	}
	return &ttsProcessor{
		cfg:        cfg,
		splitter:   newSentenceSplitter(mr),
		sentenceCh: make(chan sentenceItem, qs),
		resultCh:   make(chan synthResult, maxC),
		playbackCh: make(chan TTSChunk, 1),
		sem:        make(chan struct{}, maxC),
	}, nil
}

// --- 分句器 ---

type sentenceSplitter struct {
	buf          []rune
	firstFlushed bool
	maxRunes     int
}

func newSentenceSplitter(maxRunes int) *sentenceSplitter {
	return &sentenceSplitter{maxRunes: maxRunes}
}

func isStrongBoundary(r rune) bool {
	switch r {
	case '\n', '.', '!', '?', '。', '！', '？', '…':
		return true
	}
	return false
}

func isWeakBoundary(r rune) bool {
	switch r {
	case ',', '，', '、', ';', '；':
		return true
	}
	return false
}

func (s *sentenceSplitter) feed(text string) []string {
	var out []string
	for _, r := range text {
		s.buf = append(s.buf, r)
		shouldFlush := isStrongBoundary(r) ||
			(!s.firstFlushed && isWeakBoundary(r)) ||
			(s.maxRunes > 0 && len(s.buf) >= s.maxRunes)
		if shouldFlush {
			if seg := s.flushBuf(); seg != "" {
				out = append(out, seg)
			}
		}
	}
	return out
}

func (s *sentenceSplitter) flush() string { return s.flushBuf() }

func (s *sentenceSplitter) flushBuf() string {
	if len(s.buf) == 0 {
		return ""
	}
	seg := strings.TrimSpace(string(s.buf))
	s.buf = s.buf[:0]
	if seg != "" {
		s.firstFlushed = true
	}
	return seg
}

func (s *sentenceSplitter) reset() {
	s.buf = s.buf[:0]
	s.firstFlushed = false
}

// --- 内部类型 ---

type sentenceItem struct {
	text  string
	opts  tts.SynthesisOptions
	seq   int64
	epoch int64
}

type synthResult struct {
	seq   int64
	epoch int64
	text  string
	audio []byte // nil 表示合成失败，orderer 会跳过但推进序号
}

// --- ttsProcessor ---

type ttsProcessor struct {
	cfg *TTSConfig

	// mu 保护：onChunk、started、splitter、seqNext
	mu      sync.Mutex
	onChunk func(TTSChunk)
	started bool
	splitter *sentenceSplitter
	seqNext  int64

	// epochMu 保护 epoch
	epochMu sync.Mutex
	epoch   int64

	// 流水线 channels
	sentenceCh chan sentenceItem // 待合成
	resultCh   chan synthResult  // 合成结果（无序）
	playbackCh chan TTSChunk     // 排序后待播放（容量1）
	sem        chan struct{}     // 并发合成控制

	// synthCtx：Interrupt 时独立取消合成和 orderer 的 playbackCh 写入
	synthMu     sync.Mutex
	synthCtx    context.Context
	synthCancel context.CancelFunc

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func (p *ttsProcessor) OnChunk(fn func(TTSChunk)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onChunk = fn
}

func (p *ttsProcessor) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return errorf("TTSProcessor: already started")
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.synthCtx, p.synthCancel = context.WithCancel(p.ctx)
	p.started = true

	p.wg.Add(3) // dispatcher, orderer, playbackLoop
	go p.dispatcher()
	go p.orderer()
	go p.playbackLoop()

	logging.Infof("TTSProcessor: started (maxConcurrent=%d)", cap(p.sem))
	return nil
}

func (p *ttsProcessor) Stop() error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil
	}
	cancel := p.cancel
	p.started = false
	p.mu.Unlock()

	cancel()
	p.wg.Wait()

	logging.Infof("TTSProcessor: stopped")
	return nil
}

func (p *ttsProcessor) Write(text string, opts tts.SynthesisOptions) error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return errorf("TTSProcessor: not started")
	}
	ctx := p.ctx
	sentences := p.splitter.feed(text)
	p.mu.Unlock()

	for _, s := range sentences {
		if err := p.enqueue(ctx, s, opts); err != nil {
			return err
		}
	}
	return nil
}

func (p *ttsProcessor) Flush(opts tts.SynthesisOptions) error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return errorf("TTSProcessor: not started")
	}
	ctx := p.ctx
	seg := p.splitter.flush()
	p.mu.Unlock()

	if seg != "" {
		return p.enqueue(ctx, seg, opts)
	}
	return nil
}

func (p *ttsProcessor) Interrupt() error {
	// 1. 递增 epoch（使旧 epoch 的合成结果被 orderer 丢弃）
	p.epochMu.Lock()
	p.epoch++
	p.epochMu.Unlock()

	// 2. 取消当前合成 + orderer 等待写 playbackCh，重建 synthCtx
	p.synthMu.Lock()
	p.synthCancel()
	p.synthCtx, p.synthCancel = context.WithCancel(p.ctx)
	p.synthMu.Unlock()

	// 3. 清空各队列
	drainCh(p.sentenceCh)
	drainCh(p.resultCh)
	drainCh(p.playbackCh)

	// 4. 重置分句器和序号
	p.mu.Lock()
	p.seqNext = 0
	p.splitter.reset()
	p.mu.Unlock()

	logging.Infof("TTSProcessor: interrupted")
	return nil
}

func drainCh[T any](ch chan T) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func (p *ttsProcessor) enqueue(ctx context.Context, text string, opts tts.SynthesisOptions) error {
	if !hasSpeakableText(text) {
		return nil
	}
	p.mu.Lock()
	seq := p.seqNext
	p.seqNext++
	p.mu.Unlock()

	p.epochMu.Lock()
	epoch := p.epoch
	p.epochMu.Unlock()

	select {
	case p.sentenceCh <- sentenceItem{text: text, opts: opts, seq: seq, epoch: epoch}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- dispatcher：从 sentenceCh 取句子，并发启动 worker ---

func (p *ttsProcessor) dispatcher() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case item, ok := <-p.sentenceCh:
			if !ok {
				return
			}
			select {
			case p.sem <- struct{}{}:
			case <-p.ctx.Done():
				return
			}
			p.wg.Add(1)
			go p.worker(item)
		}
	}
}

// --- worker：合成单句，结果放入 resultCh ---

func (p *ttsProcessor) worker(item sentenceItem) {
	defer p.wg.Done()
	defer func() { <-p.sem }()

	p.synthMu.Lock()
	synthCtx := p.synthCtx
	p.synthMu.Unlock()

	reader, err := p.cfg.Provider.Synthesize(synthCtx, item.text, item.opts)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			logging.Errorf("TTSProcessor: synthesize %q: %v", truncate(item.text, 20), err)
		}
		p.sendResult(synthResult{seq: item.seq, epoch: item.epoch, text: item.text})
		return
	}
	defer func() { _ = reader.Close() }()

	audio, err := io.ReadAll(reader)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			logging.Errorf("TTSProcessor: read audio %q: %v", truncate(item.text, 20), err)
		}
		p.sendResult(synthResult{seq: item.seq, epoch: item.epoch, text: item.text})
		return
	}

	p.sendResult(synthResult{seq: item.seq, epoch: item.epoch, text: item.text, audio: audio})
}

func (p *ttsProcessor) sendResult(r synthResult) {
	p.synthMu.Lock()
	synthCtx := p.synthCtx
	p.synthMu.Unlock()

	select {
	case p.resultCh <- r:
	case <-synthCtx.Done():
	case <-p.ctx.Done():
	}
}

// --- orderer：按 seqNum 排序，顺序放入 playbackCh ---

func (p *ttsProcessor) orderer() {
	defer p.wg.Done()

	pending := make(map[int64]synthResult)
	var seqPlay int64
	var curEpoch int64

	for {
		select {
		case <-p.ctx.Done():
			return
		case result, ok := <-p.resultCh:
			if !ok {
				return
			}

			if result.epoch < curEpoch {
				continue // 旧 epoch，丢弃
			}
			if result.epoch > curEpoch {
				// 新 epoch（Interrupt 后），重置排序状态
				pending = make(map[int64]synthResult)
				seqPlay = 0
				curEpoch = result.epoch
			}

			pending[result.seq] = result

			// 按序号顺序推送到 playbackCh
			interrupted := false
			for !interrupted {
				r, ok := pending[seqPlay]
				if !ok {
					break
				}
				delete(pending, seqPlay)
				seqPlay++

				if len(r.audio) == 0 {
					continue // 合成失败的句子跳过，但序号继续推进
				}

				p.synthMu.Lock()
				synthCtx := p.synthCtx
				p.synthMu.Unlock()

				select {
				case p.playbackCh <- TTSChunk{Text: r.text, Audio: r.audio}:
				case <-synthCtx.Done():
					// Interrupt：清空 pending，不再向 playbackCh 写入
					pending = make(map[int64]synthResult)
					interrupted = true
				case <-p.ctx.Done():
					return
				}
			}
		}
	}
}

// --- playbackLoop：顺序播放 ---

func (p *ttsProcessor) playbackLoop() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case chunk, ok := <-p.playbackCh:
			if !ok {
				return
			}
			p.mu.Lock()
			fn := p.onChunk
			p.mu.Unlock()
			if fn != nil {
				fn(chunk)
			}
		}
	}
}

// --- 工具 ---

func hasSpeakableText(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
