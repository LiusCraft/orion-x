package audio

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/liuscraft/orion-x/internal/audio/resampler"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/provider/tts"
)

// PlaybackFinishedCallback is invoked when the TTS playback queue empties.
type PlaybackFinishedCallback func()

// TTSItemStartedCallback is invoked when a TTS item begins playback.
type TTSItemStartedCallback func(text string, emotion string)

// PipelineStats holds TTS processor runtime statistics.
type PipelineStats struct {
	TextQueueSize   int
	TTSBufferSize   int
	IsPlaying       bool
	TotalEnqueued   int
	TotalPlayed     int
	TotalInterrupts int
}

// TTSRequest describes a text-to-speech synthesis request.
type TTSRequest struct {
	Text    string
	Emotion string
	Voice   string  // overrides VoiceMap; empty = use VoiceMap or default
	Rate    float64 // speech rate; 0 = use default
	Pitch   float64 // pitch; 0 = use default
	Volume  int     // volume; 0 = use default
}

// TTSConfig configures a TTSProcessor.
type TTSConfig struct {
	// Provider is the TTS backend. Required.
	Provider tts.Provider
	// CallConfig is the base configuration passed to Provider.Start on each call.
	CallConfig tts.Config
	// VoiceMap maps emotion names to voice IDs, overriding CallConfig.Voice per request.
	VoiceMap      map[string]string
	MaxBuffer     int // default 3
	MaxConcurrent int // default 2
	QueueSize     int // default 100
}

// DefaultTTSConfig returns a sensible default. Provider must be set by caller.
func DefaultTTSConfig() *TTSConfig {
	return &TTSConfig{
		MaxBuffer:     3,
		MaxConcurrent: 2,
		QueueSize:     100,
		VoiceMap: map[string]string{
			"happy":   "longanyang",
			"sad":     "zhichu",
			"angry":   "zhimeng",
			"calm":    "longxiaochun",
			"excited": "longanyang",
			"default": "longanyang",
		},
		CallConfig: tts.Config{
			Model:      "cosyvoice-v3-flash",
			Voice:      "longanyang",
			Format:     "pcm",
			SampleRate: InternalSampleRate,
			Volume:     50,
			Rate:       1.0,
			Pitch:      1.0,
			TextType:   "PlainText",
		},
	}
}

// TTSProcessor converts text to PCM16LE 16kHz mono audio. Callers push text via
// Synthesize or the streaming API; audio bytes are delivered via OnAudio. The
// processor has no knowledge of where the audio will be played.
type TTSProcessor interface {
	// Synthesize enqueues a complete text item for synthesis. Non-blocking.
	Synthesize(req TTSRequest) error
	// BeginStream starts a streaming TTS session.
	BeginStream(req TTSRequest) error
	// WriteChunk appends text to the active streaming session.
	WriteChunk(text string) error
	// EndStream signals end of the streaming session's text input.
	EndStream() error
	// Interrupt clears all queues and stops current playback immediately.
	Interrupt() error
	// OnAudio registers the callback that receives PCM16LE 16kHz mono audio bytes.
	OnAudio(func(data []byte))
	// OnFinished registers the callback invoked when the playback queue empties.
	OnFinished(func())
	// OnItemStarted registers the callback invoked when a TTS item starts playing.
	OnItemStarted(func(text, emotion string))
	// Start initializes internal workers.
	Start(ctx context.Context) error
	// Stop shuts down the processor gracefully.
	Stop() error
	// Stats returns runtime statistics.
	Stats() PipelineStats
}

// NewTTSProcessor creates a TTSProcessor. cfg.Provider must not be nil.
func NewTTSProcessor(cfg *TTSConfig) (TTSProcessor, error) {
	return newTTSProcessor(cfg)
}

// eofNotifyReader wraps an io.Reader and signals when EOF is reached.
type eofNotifyReader struct {
	reader io.Reader
	doneCh chan struct{}
	once   sync.Once
}

func newEOFNotifyReader(r io.Reader) *eofNotifyReader {
	return &eofNotifyReader{
		reader: r,
		doneCh: make(chan struct{}),
	}
}

func (r *eofNotifyReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err != nil {
		r.once.Do(func() { close(r.doneCh) })
	}
	return n, err
}

func (r *eofNotifyReader) Done() <-chan struct{} {
	return r.doneCh
}

func (r *eofNotifyReader) Close() {
	r.once.Do(func() { close(r.doneCh) })
}

type ttsQueueItem struct {
	text    string
	emotion string
}

type ttsPlayItem struct {
	Reader     *eofNotifyReader
	OrigReader io.Reader
	Text       string
	Emotion    string
	DoneCh     chan struct{}
	StreamID   int64
	SeqNum     int64
}

type ttsProcessor struct {
	cfg *TTSConfig

	// callbacks
	onAudio     func([]byte)
	onFinished  PlaybackFinishedCallback
	onItemStart TTSItemStartedCallback

	// queues
	textQueue chan ttsQueueItem
	ttsBuffer chan *ttsPlayItem

	// concurrency
	ttsSemaphore chan struct{}

	// ordering
	nextSeqNum     int64
	nextPlaySeqNum int64
	pendingItems   map[int64]*ttsPlayItem
	pendingMu      sync.Mutex

	// streaming session
	activeStream tts.Stream
	sessionMu    sync.Mutex

	// state
	currentItem   *ttsPlayItem
	parentCtx     context.Context
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.Mutex
	interruptMu   sync.Mutex
	started       bool
	interrupting  bool
	streamCounter int64

	// stats
	totalEnqueued   int64
	totalPlayed     int64
	totalInterrupts int64
}

func newTTSProcessor(cfg *TTSConfig) (*ttsProcessor, error) {
	if cfg == nil {
		cfg = DefaultTTSConfig()
	}
	if cfg.Provider == nil {
		return nil, errors.New("TTSProcessor: Provider is required")
	}
	if cfg.MaxBuffer <= 0 {
		cfg.MaxBuffer = 3
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 2
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 100
	}
	if cfg.VoiceMap == nil {
		cfg.VoiceMap = map[string]string{"default": "longanyang"}
	}

	return &ttsProcessor{
		cfg:            cfg,
		textQueue:      make(chan ttsQueueItem, cfg.QueueSize),
		ttsBuffer:      make(chan *ttsPlayItem, cfg.MaxBuffer),
		ttsSemaphore:   make(chan struct{}, cfg.MaxConcurrent),
		nextSeqNum:     1,
		nextPlaySeqNum: 1,
		pendingItems:   make(map[int64]*ttsPlayItem),
	}, nil
}

func (p *ttsProcessor) OnAudio(fn func([]byte)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onAudio = fn
}

func (p *ttsProcessor) OnFinished(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onFinished = fn
}

func (p *ttsProcessor) OnItemStarted(fn func(text, emotion string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onItemStart = fn
}

func (p *ttsProcessor) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return errors.New("TTSProcessor: already started")
	}

	p.parentCtx = ctx
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.started = true

	p.startWorkers()

	logging.Infof("TTSProcessor: started (maxBuffer=%d, maxConcurrent=%d, queueSize=%d)",
		p.cfg.MaxBuffer, p.cfg.MaxConcurrent, p.cfg.QueueSize)
	return nil
}

func (p *ttsProcessor) startWorkers() {
	p.wg.Add(1)
	go p.textConsumer()
	p.wg.Add(1)
	go p.audioPlayer()
}

func (p *ttsProcessor) Stop() error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil
	}

	logging.Infof("TTSProcessor: stopping...")

	if p.cancel != nil {
		p.cancel()
	}
	currentItem := p.currentItem
	p.mu.Unlock()

	if currentItem != nil {
		currentItem.Reader.Close()
		if closer, ok := currentItem.OrigReader.(io.Closer); ok {
			_ = closer.Close()
		}
	}

	stopDrainer := make(chan struct{})
	drainerDone := make(chan struct{})
	go func() {
		defer close(drainerDone)
		for {
			select {
			case <-stopDrainer:
				for {
					select {
					case item := <-p.ttsBuffer:
						item.Reader.Close()
						if closer, ok := item.OrigReader.(io.Closer); ok {
							_ = closer.Close()
						}
					default:
						return
					}
				}
			case item := <-p.ttsBuffer:
				item.Reader.Close()
				if closer, ok := item.OrigReader.(io.Closer); ok {
					_ = closer.Close()
				}
			}
		}
	}()

	p.wg.Wait()
	close(stopDrainer)
	<-drainerDone

	p.clearQueues()

	p.mu.Lock()
	p.started = false
	p.mu.Unlock()

	logging.Infof("TTSProcessor: stopped")
	return nil
}

func (p *ttsProcessor) Synthesize(req TTSRequest) error {
	if req.Text == "" {
		return nil
	}

	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return errors.New("TTSProcessor: not started")
	}
	ctx := p.ctx
	p.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.textQueue <- ttsQueueItem{text: req.Text, emotion: req.Emotion}:
		atomic.AddInt64(&p.totalEnqueued, 1)
		return nil
	}
}

func (p *ttsProcessor) BeginStream(req TTSRequest) error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return errors.New("TTSProcessor: not started")
	}
	ctx := p.ctx
	p.mu.Unlock()

	cfg := p.cfg.CallConfig
	cfg.Voice = p.resolveVoice(req.Emotion)
	if req.Voice != "" {
		cfg.Voice = req.Voice
	}

	stream, err := p.cfg.Provider.Start(ctx, cfg)
	if err != nil {
		return err
	}

	p.sessionMu.Lock()
	p.activeStream = stream
	p.sessionMu.Unlock()

	audioReader := stream.AudioReader()
	var reader io.Reader = audioReader
	if ttsSR := stream.SampleRate(); ttsSR != InternalSampleRate {
		r := resampler.NewLinearResampler()
		reader = resampler.NewResamplingReader(audioReader, ttsSR, InternalSampleRate, stream.Channels(), r)
	}

	streamID := atomic.AddInt64(&p.streamCounter, 1)
	notifyReader := newEOFNotifyReader(reader)
	item := &ttsPlayItem{
		Reader:     notifyReader,
		OrigReader: audioReader,
		Text:       "[stream]",
		Emotion:    req.Emotion,
		DoneCh:     make(chan struct{}),
		StreamID:   streamID,
	}

	select {
	case p.ttsBuffer <- item:
		atomic.AddInt64(&p.totalEnqueued, 1)
		return nil
	case <-ctx.Done():
		_ = stream.Close(ctx)
		return ctx.Err()
	}
}

func (p *ttsProcessor) WriteChunk(text string) error {
	p.sessionMu.Lock()
	stream := p.activeStream
	p.sessionMu.Unlock()

	if stream == nil {
		return errors.New("TTSProcessor: no active stream session")
	}

	p.mu.Lock()
	ctx := p.ctx
	p.mu.Unlock()

	return stream.WriteTextChunk(ctx, text)
}

func (p *ttsProcessor) EndStream() error {
	p.sessionMu.Lock()
	stream := p.activeStream
	p.activeStream = nil
	p.sessionMu.Unlock()

	if stream == nil {
		return nil
	}

	p.mu.Lock()
	ctx := p.ctx
	p.mu.Unlock()

	return stream.Finish(ctx)
}

func (p *ttsProcessor) Interrupt() error {
	p.interruptMu.Lock()
	defer p.interruptMu.Unlock()

	p.mu.Lock()
	if p.interrupting || !p.started {
		p.mu.Unlock()
		return nil
	}
	p.interrupting = true
	p.mu.Unlock()

	logging.Infof("TTSProcessor: interrupting...")

	p.sessionMu.Lock()
	p.activeStream = nil
	p.sessionMu.Unlock()

	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	currentItem := p.currentItem
	p.mu.Unlock()

	if currentItem != nil {
		currentItem.Reader.Close()
		if closer, ok := currentItem.OrigReader.(io.Closer); ok {
			_ = closer.Close()
		}
	}

	p.wg.Wait()
	p.clearQueues()

	p.pendingMu.Lock()
	p.nextSeqNum = 1
	p.nextPlaySeqNum = 1
	p.pendingItems = make(map[int64]*ttsPlayItem)
	p.pendingMu.Unlock()

	p.mu.Lock()
	if p.parentCtx != nil && p.parentCtx.Err() == nil {
		p.ctx, p.cancel = context.WithCancel(p.parentCtx)
		p.startWorkers()
	}
	p.interrupting = false
	atomic.AddInt64(&p.totalInterrupts, 1)
	p.mu.Unlock()

	logging.Infof("TTSProcessor: interrupt completed")
	return nil
}

func (p *ttsProcessor) Stats() PipelineStats {
	p.mu.Lock()
	isPlaying := p.currentItem != nil
	p.mu.Unlock()

	return PipelineStats{
		TextQueueSize:   len(p.textQueue),
		TTSBufferSize:   len(p.ttsBuffer),
		IsPlaying:       isPlaying,
		TotalEnqueued:   int(atomic.LoadInt64(&p.totalEnqueued)),
		TotalPlayed:     int(atomic.LoadInt64(&p.totalPlayed)),
		TotalInterrupts: int(atomic.LoadInt64(&p.totalInterrupts)),
	}
}

func (p *ttsProcessor) textConsumer() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case item := <-p.textQueue:
			p.pendingMu.Lock()
			seqNum := p.nextSeqNum
			p.nextSeqNum++
			p.pendingMu.Unlock()

			p.wg.Add(1)
			go p.ttsWorker(item, seqNum)
		}
	}
}

func (p *ttsProcessor) ttsWorker(item ttsQueueItem, seqNum int64) {
	defer p.wg.Done()

	select {
	case <-p.ctx.Done():
		p.notifySeqCompleted(seqNum, nil)
		return
	case p.ttsSemaphore <- struct{}{}:
		defer func() { <-p.ttsSemaphore }()
	}

	streamID := atomic.AddInt64(&p.streamCounter, 1)

	reader, err := p.generateTTS(p.ctx, item.text, item.emotion)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			logging.Errorf("TTSProcessor: [stream-%d seq-%d] TTS error: %v", streamID, seqNum, err)
		}
		p.notifySeqCompleted(seqNum, nil)
		return
	}

	notifyReader := newEOFNotifyReader(reader)
	playItem := &ttsPlayItem{
		Reader:     notifyReader,
		OrigReader: reader,
		Text:       item.text,
		Emotion:    item.emotion,
		DoneCh:     make(chan struct{}),
		StreamID:   streamID,
		SeqNum:     seqNum,
	}

	p.notifySeqCompleted(seqNum, playItem)
}

func (p *ttsProcessor) notifySeqCompleted(seqNum int64, item *ttsPlayItem) {
	p.pendingMu.Lock()

	if item != nil {
		p.pendingItems[seqNum] = item
	}

	var toSend []*ttsPlayItem
	for {
		next, ok := p.pendingItems[p.nextPlaySeqNum]
		if !ok {
			break
		}
		delete(p.pendingItems, p.nextPlaySeqNum)
		p.nextPlaySeqNum++
		if next != nil {
			toSend = append(toSend, next)
		}
	}
	p.pendingMu.Unlock()

	for _, itm := range toSend {
		select {
		case <-p.ctx.Done():
			itm.Reader.Close()
			if closer, ok := itm.OrigReader.(io.Closer); ok {
				_ = closer.Close()
			}
			return
		case p.ttsBuffer <- itm:
		}
	}
}

func (p *ttsProcessor) audioPlayer() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case item := <-p.ttsBuffer:
			p.playItem(item)
		}
	}
}

func (p *ttsProcessor) playItem(item *ttsPlayItem) {
	p.mu.Lock()
	p.currentItem = item
	audioCallback := p.onAudio
	startedCallback := p.onItemStart
	p.mu.Unlock()

	if startedCallback != nil {
		startedCallback(item.Text, item.Emotion)
	}

	if audioCallback != nil {
		buf := make([]byte, 4096)
		for {
			select {
			case <-p.ctx.Done():
				item.Reader.Close()
				if closer, ok := item.OrigReader.(io.Closer); ok {
					_ = closer.Close()
				}
				goto cleanup
			default:
			}
			n, err := item.Reader.Read(buf)
			if n > 0 {
				audioCallback(buf[:n])
			}
			if err != nil {
				break
			}
		}
	}

	if closer, ok := item.OrigReader.(io.Closer); ok {
		_ = closer.Close()
	}

cleanup:
	p.mu.Lock()
	p.currentItem = nil
	finishedCallback := p.onFinished
	p.mu.Unlock()

	atomic.AddInt64(&p.totalPlayed, 1)
	close(item.DoneCh)

	if finishedCallback != nil {
		finishedCallback()
	}
}

func (p *ttsProcessor) generateTTS(ctx context.Context, text string, emotion string) (io.Reader, error) {
	cfg := p.cfg.CallConfig
	cfg.Voice = p.resolveVoice(emotion)

	ttsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	stream, err := p.cfg.Provider.Start(ttsCtx, cfg)
	if err != nil {
		return nil, err
	}

	if err := stream.WriteTextChunk(ttsCtx, text); err != nil {
		_ = stream.Close(ttsCtx)
		return nil, err
	}

	if err := stream.Close(ttsCtx); err != nil {
		return nil, err
	}

	audioReader := stream.AudioReader()
	if ttsSR := stream.SampleRate(); ttsSR != InternalSampleRate {
		r := resampler.NewLinearResampler()
		return resampler.NewResamplingReader(audioReader, ttsSR, InternalSampleRate, stream.Channels(), r), nil
	}
	return audioReader, nil
}

func (p *ttsProcessor) resolveVoice(emotion string) string {
	if emotion != "" {
		if v, ok := p.cfg.VoiceMap[emotion]; ok {
			return v
		}
	}
	if v, ok := p.cfg.VoiceMap["default"]; ok {
		return v
	}
	if p.cfg.CallConfig.Voice != "" {
		return p.cfg.CallConfig.Voice
	}
	return "longanyang"
}

func (p *ttsProcessor) clearQueues() {
	for {
		select {
		case <-p.textQueue:
		default:
			goto clearPending
		}
	}

clearPending:
	p.pendingMu.Lock()
	for _, item := range p.pendingItems {
		if item != nil {
			item.Reader.Close()
			if closer, ok := item.OrigReader.(io.Closer); ok {
				_ = closer.Close()
			}
		}
	}
	p.pendingItems = make(map[int64]*ttsPlayItem)
	p.pendingMu.Unlock()

	for {
		select {
		case item := <-p.ttsBuffer:
			item.Reader.Close()
			if closer, ok := item.OrigReader.(io.Closer); ok {
				_ = closer.Close()
			}
		default:
			return
		}
	}
}

func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen])
}
