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
	textutil "github.com/liuscraft/orion-x/internal/text"
)

// TTSChunk 是一个文本-音频配对单元，对应一句话。
type TTSChunk struct {
	Text  string
	Audio []byte
	// Final 标记本轮最后一块音频，此时 Text/Audio 均为空。
	Final bool
	// SentenceEnd 标记该 chunk 音频包含某句话的最后一个字节。
	SentenceEnd  bool
	SentenceText string
}

// TTSConfig 配置 TTSProcessor。
type TTSConfig struct {
	// Provider 是 TTS 后端。必填。
	Provider tts.Synthesizer
	// QueueSize 是待合成句子队列的大小，默认 100。
	QueueSize int
	// MaxRunes 是单句最大字符数（超过则强制切句），默认 100。
	MaxRunes int
	// BaseRequest 是合成的默认参数，Write/Flush 的 opts 会覆盖其中跟文本无关的字段。
	BaseRequest tts.SynthesizeRequest
}

// streamFlushTimeout 是 handleFlush 等待 streaming 路径音频播放完成的上限。
const streamFlushTimeout = 5 * time.Second

// DefaultTTSConfig 返回合理的默认配置，Provider 必须由调用方设置。
func DefaultTTSConfig() *TTSConfig {
	return &TTSConfig{
		QueueSize: 100,
		MaxRunes:  100,
		BaseRequest: tts.SynthesizeRequest{
			Speech: tts.SpeechParams{Speed: 1.0, Pitch: 1.0},
		},
	}
}

// TTSProcessor 接收 LLM 流式文本，内部分句后合成音频，
// 以音频帧为单位通过 OnChunk 回调。
//
// StreamingSynthesizer 路径（per-turn stream）：
//
//	Write → splitter → sentenceCh → [dispatcher] → WriteTextChunk → currentStream → AudioReader → onChunk
//	Flush → currentStream.Finish()
//
// 非 StreamingSynthesizer 路径（fallback，串行）：
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
		baseReq:      cfg.BaseRequest,
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
	req   tts.SynthesizeRequest
	flush bool // true 表示 Flush 信号，dispatcher 需调 currentStream.Finish()
}

// --- ttsProcessor ---

type ttsProcessor struct {
	cfg     *TTSConfig
	baseReq tts.SynthesizeRequest

	// mu 保护：onChunk、started、splitter、turnStarted、currentEmotion
	mu             sync.Mutex
	onChunk        func(TTSChunk)
	started        bool
	splitter       *sentenceSplitter
	turnStarted    bool   // 本轮第一次 Write 后置 true，Interrupt 后复位
	currentEmotion string // 当前情绪 emoji，由文本中首 emoji 驱动，Interrupt 复位

	// provider 能力，Start() 时缓存，生命周期内不变
	streamingProv tts.StreamingSynthesizer
	warmableProv  tts.WarmableProvider

	// sentenceCh：Write → dispatcher 的句子队列
	sentenceCh chan sentenceItem

	// synthCtx：Interrupt 时取消，中断合成和音频读取
	synthMu     sync.Mutex
	synthCtx    context.Context
	synthCancel context.CancelFunc

	// currentStream：per-turn stream（StreamingSynthesizer 路径），turn 结束（Flush）后置 nil
	streamMu      sync.Mutex
	currentStream tts.SynthesisStream
	streamDoneCh  chan struct{} // 当前 stream 的 playAudio 读到 EOF 时关闭

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
	p.streamingProv, _ = p.cfg.Provider.(tts.StreamingSynthesizer)
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
	p.streamDoneCh = nil
	p.streamMu.Unlock()

	cancel()
	p.wg.Wait()

	logging.Infof("TTSProcessor: stopped")
	return nil
}

// buildReq 将旧 SynthesisOptions 合并到 baseReq 构建 SynthesizeRequest。
func (p *ttsProcessor) buildReq(text string, opts tts.SynthesisOptions) tts.SynthesizeRequest {
	req := p.baseReq
	req.Input = tts.TextInput{Text: text}
	if opts.Rate > 0 {
		req.Speech.Speed = opts.Rate
	}
	if opts.Emotion != "" {
		req.Speech.Emotion = opts.Emotion
	}
	return req
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
		warmReq := p.buildReq("", opts) // 预热不需要 text
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			stream := p.warmableProv.Warm(synthCtx, warmReq)
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
		if emo, stripped := textutil.ExtractLeadingEmoji(s); emo != "" {
			p.currentEmotion = emo
			s = stripped
		}
		if !hasSpeakableText(s) {
			continue
		}
		finalEmotion := p.currentEmotion
		itemOpts := opts
		if finalEmotion != "" {
			itemOpts.Emotion = finalEmotion
		}
		item := sentenceItem{
			text: s,
			req:  p.buildReq(s, itemOpts),
		}
		select {
		case p.sentenceCh <- item:
			logging.Infof("TTSProcessor: sentence enqueued (text=%q, emotion=%s)", truncate(s, 30), p.currentEmotion)
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

	if seg != "" {
		if emo, stripped := textutil.ExtractLeadingEmoji(seg); emo != "" {
			p.currentEmotion = emo
			seg = stripped
		}
	}
	if seg != "" && hasSpeakableText(seg) {
		finalEmotion := p.currentEmotion
		itemOpts := opts
		if finalEmotion != "" {
			itemOpts.Emotion = finalEmotion
		}
		item := sentenceItem{
			text: seg,
			req:  p.buildReq(seg, itemOpts),
		}
		select {
		case p.sentenceCh <- item:
			logging.Infof("TTSProcessor: sentence enqueued (flush, text=%q, emotion=%s)", truncate(seg, 30), p.currentEmotion)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// flush 信号：dispatcher 收到后调 currentStream.Finish()
	select {
	case p.sentenceCh <- sentenceItem{flush: true, req: p.buildReq("", opts)}:
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
	p.streamDoneCh = nil
	p.streamMu.Unlock()

	// 4. 清空句子队列
	drainSentenceCh(p.sentenceCh)

	// 5. 重置分句器、turn 状态和情绪
	p.mu.Lock()
	p.splitter.reset()
	p.turnStarted = false
	p.currentEmotion = ""
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
				p.dispatchStreaming(synthCtx, item)
			} else {
				p.dispatchBatch(synthCtx, item)
			}
		}
	}
}

// handleFlush 在 dispatcher 里处理 Flush 信号：向 currentStream 发 finish-task，
// 等待其音频播放完成（streaming 路径），最后发出 Final 标记 chunk。
func (p *ttsProcessor) handleFlush(synthCtx context.Context) {
	p.streamMu.Lock()
	stream := p.currentStream
	doneCh := p.streamDoneCh
	p.currentStream = nil
	p.streamDoneCh = nil
	p.streamMu.Unlock()

	if stream != nil {
		if err := stream.Finish(synthCtx); err != nil && !errors.Is(err, context.Canceled) {
			logging.Errorf("TTSProcessor: Finish failed: %v", err)
		}
		if doneCh != nil {
			select {
			case <-doneCh:
			case <-synthCtx.Done():
			case <-time.After(streamFlushTimeout):
				logging.Warnf("TTSProcessor: timeout waiting stream playback to finish after Finish")
			}
		}
	}

	p.mu.Lock()
	fn := p.onChunk
	p.mu.Unlock()
	if fn != nil {
		fn(TTSChunk{Final: true})
	}
}

// dispatchStreaming 走 per-turn stream 路径：复用 currentStream，WriteTextChunk。
func (p *ttsProcessor) dispatchStreaming(synthCtx context.Context, item sentenceItem) {
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
			stream, err = p.streamingProv.StartSynthesis(synthCtx, item.req)
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
		boundaries := stream.SentenceBoundaries()
		doneCh := make(chan struct{})
		p.streamMu.Lock()
		p.streamDoneCh = doneCh
		p.streamMu.Unlock()
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.playAudio(reader, "", synthCtx, func() { close(doneCh) }, boundaries)
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
func (p *ttsProcessor) dispatchBatch(synthCtx context.Context, item sentenceItem) {
	start := time.Now()
	result, err := p.cfg.Provider.Synthesize(synthCtx, item.req)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			logging.Errorf("TTSProcessor: Synthesize %q failed after %v: %v", truncate(item.text, 20), time.Since(start), err)
		}
		return
	}
	logging.Infof("TTSProcessor: Synthesize done (elapsed=%v, text=%q)", time.Since(start), truncate(item.text, 30))
	p.playAudio(result.Audio, item.text, synthCtx, nil, nil)
}

// playAudio 从 reader 流式读 PCM，每帧调 onChunk。firstText 只在首帧附加。
func (p *ttsProcessor) playAudio(reader io.ReadCloser, firstText string, synthCtx context.Context, onEOF func(), boundaries <-chan tts.SentenceBoundary) {
	var pendingBoundary *tts.SentenceBoundary
	nextText := firstText
	defer func() { _ = reader.Close() }()

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

			// Non-blocking drain of pending boundaries.
			for {
				select {
				case b, ok := <-boundaries:
					if !ok {
						break
					}
					if b.IsBegin {
						nextText = b.Text
						continue
					}
					pendingBoundary = &b
				default:
				}
				break
			}

			text := ""
			if first {
				first = false
				logging.Infof("TTSProcessor: playback first frame (elapsed=%v, frame_bytes=%d)", time.Since(playStart), n)
			}
			if nextText != "" {
				text = nextText
				nextText = ""
			}

			audioCopy := make([]byte, n)
			copy(audioCopy, buf[:n])
			chunk := TTSChunk{Text: text, Audio: audioCopy}
			if pendingBoundary != nil && totalBytes >= pendingBoundary.Offset {
				chunk.SentenceEnd = true
				chunk.SentenceText = pendingBoundary.Text
				pendingBoundary = nil
			}
			if fn != nil {
				fn(chunk)
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

	if onEOF != nil && synthCtx.Err() == nil {
		onEOF()
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
