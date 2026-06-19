package audio

import (
	"context"
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
	Text  string // 句子文本
	Audio []byte // 该句完整 PCM 音频
}

// TTSConfig 配置 TTSProcessor。
type TTSConfig struct {
	// Provider 是 TTS 后端。必填。
	Provider tts.Provider
	// MaxConcurrent 是并发合成的最大句数，默认 2。
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

// TTSProcessor 接收 LLM 流式文本，内部分句后逐句合成音频，
// 以句为单位通过 OnChunk 回调文本+音频配对。
type TTSProcessor interface {
	// Write 持续送入 LLM 流式 token。内部按停顿分句，首句用弱停顿，后续用强停顿。
	Write(text string, opts tts.SynthesisOptions) error
	// Flush 强制输出剩余缓冲文本（LLM 输出结束时调用）。
	Flush(opts tts.SynthesisOptions) error
	// OnChunk 注册回调，每句话合成完时收到文本+音频配对。
	OnChunk(func(TTSChunk))
	// Interrupt 清空待合成队列，丢弃进行中的合成结果。
	Interrupt() error
	// Start 启动内部 worker。
	Start(ctx context.Context) error
	// Stop 停止并等待所有 goroutine 退出。
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
		sem:        make(chan struct{}, maxC),
		pending:    make(map[int64]TTSChunk),
	}, nil
}

// --- 分句器 ---

// sentenceSplitter 两阶段分句：首句在弱停顿处切，后续在强停顿处切。
type sentenceSplitter struct {
	buf          []rune
	firstFlushed bool // 首句是否已输出
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

func (s *sentenceSplitter) flush() string {
	return s.flushBuf()
}

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

// --- 待合成队列项 ---

type sentenceItem struct {
	text  string
	opts  tts.SynthesisOptions
	seq   int64
	epoch int64
}

// --- ttsProcessor ---

type ttsProcessor struct {
	cfg *TTSConfig

	mu      sync.Mutex
	onChunk func(TTSChunk)
	started bool

	splitter   *sentenceSplitter
	sentenceCh chan sentenceItem
	sem        chan struct{}

	// 顺序保证
	pendingMu sync.Mutex
	pending   map[int64]TTSChunk
	seqNext   int64
	seqPlay   int64
	epoch     int64

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
	p.started = true

	p.wg.Add(1)
	go p.dispatcher()

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

// hasSpeakableText 判断文本是否含有可朗读的字符（字母、数字、汉字等），
// 过滤掉纯 emoji / 标点 / 空白的句子，避免无效的 TTS 请求。
func hasSpeakableText(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func (p *ttsProcessor) enqueue(ctx context.Context, text string, opts tts.SynthesisOptions) error {
	if !hasSpeakableText(text) {
		return nil
	}
	p.pendingMu.Lock()
	seq := p.seqNext
	p.seqNext++
	epoch := p.epoch
	p.pendingMu.Unlock()

	select {
	case p.sentenceCh <- sentenceItem{text: text, opts: opts, seq: seq, epoch: epoch}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *ttsProcessor) Interrupt() error {
	// 递增 epoch，使正在进行的合成结果被丢弃
	p.pendingMu.Lock()
	p.epoch++
	p.seqNext = 0
	p.seqPlay = 0
	p.pending = make(map[int64]TTSChunk)
	p.pendingMu.Unlock()

	// 清空待合成队列
loop:
	for {
		select {
		case <-p.sentenceCh:
		default:
			break loop
		}
	}

	// 重置分句器（与 Write/Flush 共用 mu 保护）
	p.mu.Lock()
	p.splitter.reset()
	p.mu.Unlock()

	logging.Infof("TTSProcessor: interrupted")
	return nil
}

// --- dispatcher & worker ---

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
			// 阻塞等待 semaphore（控制并发合成数量）
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

func (p *ttsProcessor) worker(item sentenceItem) {
	defer p.wg.Done()
	defer func() { <-p.sem }()

	reader, err := p.cfg.Provider.Synthesize(p.ctx, item.text, item.opts)
	if err != nil {
		logging.Errorf("TTSProcessor: synthesize %q error: %v", truncate(item.text, 20), err)
		p.notifyDone(item, nil)
		return
	}
	defer func() { _ = reader.Close() }()

	audio, err := io.ReadAll(reader)
	if err != nil {
		logging.Errorf("TTSProcessor: read audio for %q error: %v", truncate(item.text, 20), err)
		p.notifyDone(item, nil)
		return
	}

	p.notifyDone(item, audio)
}

func (p *ttsProcessor) notifyDone(item sentenceItem, audio []byte) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()

	// 已被 Interrupt，丢弃
	if item.epoch != p.epoch {
		return
	}

	p.pending[item.seq] = TTSChunk{Text: item.text, Audio: audio}

	p.mu.Lock()
	fn := p.onChunk
	p.mu.Unlock()

	// 按序号顺序回调
	for {
		chunk, ok := p.pending[p.seqPlay]
		if !ok {
			break
		}
		delete(p.pending, p.seqPlay)
		p.seqPlay++

		if fn != nil && len(chunk.Audio) > 0 {
			fn(chunk)
		}
	}
}

// --- 工具函数 ---

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
