package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
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
	// QueueSize 是待合成句子队列的大小，默认 100。
	QueueSize int
	// MaxRunes 是单句最大字符数（超过则强制切句），默认 100。
	MaxRunes int
}

// DefaultTTSConfig 返回合理的默认配置，Provider 必须由调用方设置。
func DefaultTTSConfig() *TTSConfig {
	return &TTSConfig{
		QueueSize: 100,
		MaxRunes:  100,
	}
}

// TTSProcessor 接收 LLM 流式文本，内部分句后合成音频，
// 以音频帧为单位通过 OnChunk 回调。
//
// StreamingProvider 路径（per-turn stream）：
//
//	Write → splitter → sentenceCh → [dispatcher] → WriteTextChunk → currentStream → AudioReader → onChunk
//	Flush → currentStream.Finish()
//
// 非 StreamingProvider 路径（fallback，串行）：
//
//	Write → splitter → sentenceCh → [dispatcher] → Synthesize → onChunk
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
	qs := cfg.QueueSize
	if qs <= 0 {
		qs = 100
	}
	mr := cfg.MaxRunes
	if mr <= 0 {
		mr = 100
	}
	return &ttsProcessor{
		cfg:          cfg,
		splitter:     newSentenceSplitter(mr),
		sentenceCh:   make(chan sentenceItem, qs),
		warmResultCh: make(chan tts.SynthesisStream, 1),
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
	flush bool // true 表示 Flush 信号，dispatcher 需调 currentStream.Finish()
}

// --- ttsProcessor ---

type ttsProcessor struct {
	cfg *TTSConfig

	// mu 保护：onChunk、started、splitter、turnStarted
	mu          sync.Mutex
	onChunk     func(TTSChunk)
	started     bool
	splitter    *sentenceSplitter
	turnStarted bool // 本轮第一次 Write 后置 true，Interrupt 后复位

	// provider 能力，Start() 时缓存，生命周期内不变
	streamingProv tts.StreamingProvider
	warmableProv  tts.WarmableProvider

	// sentenceCh：Write → dispatcher 的句子队列
	sentenceCh chan sentenceItem

	// synthCtx：Interrupt 时取消，中断合成和音频读取
	synthMu     sync.Mutex
	synthCtx    context.Context
	synthCancel context.CancelFunc

	// currentStream：per-turn stream（StreamingProvider 路径），turn 结束（Flush）后置 nil
	streamMu      sync.Mutex
	currentStream tts.SynthesisStream

	// warmResultCh：Warm goroutine 完成后把就绪 stream 送到这里（容量1）
	warmResultCh chan tts.SynthesisStream

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
	p.streamingProv, _ = p.cfg.Provider.(tts.StreamingProvider)
	p.warmableProv, _ = p.cfg.Provider.(tts.WarmableProvider)

	p.wg.Add(1)
	go p.dispatcher()

	logging.Infof("TTSProcessor: started")
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

	// 关闭当前 stream 让 streamAudio goroutine 退出
	p.streamMu.Lock()
	if p.currentStream != nil {
		p.currentStream.Abort()
		p.currentStream = nil
	}
	p.streamMu.Unlock()

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
	firstWrite := !p.turnStarted
	if firstWrite {
		p.turnStarted = true
	}
	sentences := p.splitter.feed(text)
	p.mu.Unlock()

	// 本轮第一次 Write：在 goroutine 里预热连接，结果送 warmResultCh 给 dispatcher 消费。
	if firstWrite && p.warmableProv != nil {
		p.synthMu.Lock()
		synthCtx := p.synthCtx
		p.synthMu.Unlock()
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			stream := p.warmableProv.Warm(synthCtx, opts)
			if stream == nil {
				return
			}
			select {
			case p.warmResultCh <- stream:
			case <-synthCtx.Done():
				stream.Abort()
			case <-p.ctx.Done():
				stream.Abort()
			}
		}()
	}

	for _, s := range sentences {
		if !hasSpeakableText(s) {
			continue
		}
		select {
		case p.sentenceCh <- sentenceItem{text: s, opts: opts}:
			logging.Infof("TTSProcessor: sentence enqueued (text=%q)", truncate(s, 30))
		case <-ctx.Done():
			return ctx.Err()
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

	if seg != "" && hasSpeakableText(seg) {
		select {
		case p.sentenceCh <- sentenceItem{text: seg, opts: opts}:
			logging.Infof("TTSProcessor: sentence enqueued (flush, text=%q)", truncate(seg, 30))
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// flush 信号：dispatcher 收到后调 currentStream.Finish()
	select {
	case p.sentenceCh <- sentenceItem{flush: true, opts: opts}:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (p *ttsProcessor) Interrupt() error {
	// 1. 取消当前合成和音频读取，重建 synthCtx
	p.synthMu.Lock()
	p.synthCancel()
	p.synthCtx, p.synthCancel = context.WithCancel(p.ctx)
	p.synthMu.Unlock()

	// 2. drain warmResultCh，Abort 尚未被消费的预热 stream
	select {
	case s := <-p.warmResultCh:
		s.Abort()
	default:
	}

	// 3. 中止当前 stream（关闭 audioBuf 让 streamAudio goroutine 退出）
	p.streamMu.Lock()
	if p.currentStream != nil {
		p.currentStream.Abort()
		p.currentStream = nil
	}
	p.streamMu.Unlock()

	// 4. 清空句子队列
	drainSentenceCh(p.sentenceCh)

	// 5. 重置分句器和 turn 状态
	p.mu.Lock()
	p.splitter.reset()
	p.turnStarted = false
	p.mu.Unlock()

	logging.Infof("TTSProcessor: interrupted")
	return nil
}

func drainSentenceCh(ch chan sentenceItem) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// --- dispatcher：从 sentenceCh 串行处理句子 ---

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

			p.synthMu.Lock()
			synthCtx := p.synthCtx
			p.synthMu.Unlock()

			if item.flush {
				p.handleFlush(synthCtx)
				continue
			}

			if p.streamingProv != nil {
				p.dispatchStreaming(item, synthCtx)
			} else {
				p.dispatchBatch(item, synthCtx)
			}
		}
	}
}

// handleFlush 在 dispatcher 里处理 Flush 信号：向 currentStream 发 finish-task，重置 stream。
func (p *ttsProcessor) handleFlush(synthCtx context.Context) {
	p.streamMu.Lock()
	stream := p.currentStream
	p.currentStream = nil
	p.streamMu.Unlock()

	if stream == nil {
		return
	}
	if err := stream.Finish(synthCtx); err != nil && !errors.Is(err, context.Canceled) {
		logging.Errorf("TTSProcessor: Finish failed: %v", err)
	}
}

// dispatchStreaming 走 per-turn stream 路径：复用 currentStream，WriteTextChunk。
func (p *ttsProcessor) dispatchStreaming(item sentenceItem, synthCtx context.Context) {
	p.streamMu.Lock()
	stream := p.currentStream
	p.streamMu.Unlock()

	if stream == nil {
		// 优先等 warm stream（由 Write() goroutine 建立）。
		// warm 在第一个 token 时已触发，通常只需等几十毫秒；超时（300ms）则新建连接。
		waitCtx, cancel := context.WithTimeout(synthCtx, 300*time.Millisecond)
		select {
		case s := <-p.warmResultCh:
			stream = s
			logging.Infof("TTSProcessor: warm stream consumed")
		case <-waitCtx.Done():
		}
		cancel()

		if stream == nil {
			var err error
			stream, err = p.streamingProv.StartSynthesis(synthCtx, item.opts)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					logging.Errorf("TTSProcessor: StartSynthesis failed: %v", err)
				}
				return
			}
		}
		p.streamMu.Lock()
		p.currentStream = stream
		p.streamMu.Unlock()

		reader := stream.AudioReader()
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.playAudio(reader, "", synthCtx)
		}()
	}

	start := time.Now()
	if err := stream.WriteTextChunk(synthCtx, item.text); err != nil {
		if !errors.Is(err, context.Canceled) {
			logging.Errorf("TTSProcessor: WriteTextChunk %q failed after %v: %v", truncate(item.text, 20), time.Since(start), err)
		}
		p.streamMu.Lock()
		if p.currentStream == stream {
			p.currentStream = nil
		}
		p.streamMu.Unlock()
		stream.Abort()
		return
	}
	logging.Infof("TTSProcessor: WriteTextChunk sent (elapsed=%v, text=%q)", time.Since(start), truncate(item.text, 30))
}

// dispatchBatch 走非流式 fallback 路径：串行 Synthesize，直接在 dispatcher goroutine 里播放。
func (p *ttsProcessor) dispatchBatch(item sentenceItem, synthCtx context.Context) {
	start := time.Now()
	reader, err := p.cfg.Provider.Synthesize(synthCtx, item.text, item.opts)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			logging.Errorf("TTSProcessor: Synthesize %q failed after %v: %v", truncate(item.text, 20), time.Since(start), err)
		}
		return
	}
	logging.Infof("TTSProcessor: Synthesize done (elapsed=%v, text=%q)", time.Since(start), truncate(item.text, 30))
	p.playAudio(reader, item.text, synthCtx)
}

// playAudio 从 reader 流式读 PCM，每帧调 onChunk。firstText 只在首帧附加。
func (p *ttsProcessor) playAudio(reader io.ReadCloser, firstText string, synthCtx context.Context) {
	defer reader.Close()

	p.mu.Lock()
	fn := p.onChunk
	p.mu.Unlock()

	buf := make([]byte, 4096)
	first := true
	playStart := time.Now()
	totalBytes := 0

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			totalBytes += n

			text := ""
			if first {
				text = firstText
				first = false
				logging.Infof("TTSProcessor: playback first frame (elapsed=%v, frame_bytes=%d)", time.Since(playStart), n)
			}

			if fn != nil {
				fn(TTSChunk{Text: text, Audio: buf[:n]})
			}
		}
		if err != nil {
			break
		}

		select {
		case <-synthCtx.Done():
			return
		case <-p.ctx.Done():
			return
		default:
		}
	}

	logging.Infof("TTSProcessor: playback done (elapsed=%v, total_bytes=%d)", time.Since(playStart), totalBytes)
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
