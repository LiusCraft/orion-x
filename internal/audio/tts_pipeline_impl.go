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

// eofNotifyReader wraps an io.Reader and signals when EOF is reached
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

type ttsItem struct {
	Reader     *eofNotifyReader
	OrigReader io.Reader
	Text       string
	Emotion    string
	DoneCh     chan struct{}
	StreamID   int64
	SeqNum     int64
}

type ttsPipelineImpl struct {
	config    *TTSPipelineConfig
	provider  tts.Provider
	ttsConfig tts.Config
	voiceMap  map[string]string

	// 外部依赖（可动态设置）
	sink               AudioSink
	onPlaybackFinished PlaybackFinishedCallback
	onItemStarted      TTSItemStartedCallback

	// 队列
	textQueue chan textItem
	ttsBuffer chan *ttsItem

	// 并发控制
	ttsSemaphore chan struct{}

	// 顺序控制
	nextSeqNum     int64
	nextPlaySeqNum int64
	pendingItems   map[int64]*ttsItem
	pendingMu      sync.Mutex

	// 流式 session 状态
	activeStream tts.Stream
	sessionMu    sync.Mutex

	// 状态
	currentItem   *ttsItem
	parentCtx     context.Context
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.Mutex
	interruptMu   sync.Mutex
	started       bool
	interrupting  bool
	streamCounter int64

	// 统计
	totalEnqueued   int64
	totalPlayed     int64
	totalInterrupts int64
}

func NewTTSPipeline(
	provider tts.Provider,
	config *TTSPipelineConfig,
	ttsConfig tts.Config,
	voiceMap map[string]string,
) TTSPipeline {
	if config == nil {
		config = DefaultTTSPipelineConfig()
	}
	if voiceMap == nil {
		voiceMap = map[string]string{
			"default": "longanyang",
		}
	}

	return &ttsPipelineImpl{
		config:         config,
		provider:       provider,
		ttsConfig:      ttsConfig,
		voiceMap:       voiceMap,
		textQueue:      make(chan textItem, config.TextQueueSize),
		ttsBuffer:      make(chan *ttsItem, config.MaxTTSBuffer),
		ttsSemaphore:   make(chan struct{}, config.MaxConcurrentTTS),
		nextSeqNum:     1,
		nextPlaySeqNum: 1,
		pendingItems:   make(map[int64]*ttsItem),
	}
}

func (p *ttsPipelineImpl) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return errors.New("TTSPipeline: already started")
	}

	p.parentCtx = ctx
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.started = true

	p.startWorkers()

	logging.Infof("TTSPipeline: started (maxTTSBuffer=%d, maxConcurrent=%d, textQueueSize=%d)",
		p.config.MaxTTSBuffer, p.config.MaxConcurrentTTS, p.config.TextQueueSize)
	return nil
}

func (p *ttsPipelineImpl) startWorkers() {
	p.wg.Add(1)
	go p.textConsumer()

	p.wg.Add(1)
	go p.audioPlayer()
}

func (p *ttsPipelineImpl) Stop() error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil
	}

	logging.Infof("TTSPipeline: stopping...")

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
		drained := 0
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
						drained++
					default:
						return
					}
				}
			case item := <-p.ttsBuffer:
				item.Reader.Close()
				if closer, ok := item.OrigReader.(io.Closer); ok {
					_ = closer.Close()
				}
				drained++
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

	logging.Infof("TTSPipeline: stopped")
	return nil
}

func (p *ttsPipelineImpl) EnqueueText(text string, emotion string) error {
	if text == "" {
		return nil
	}

	p.mu.Lock()
	ctx := p.ctx
	if !p.started {
		p.mu.Unlock()
		return errors.New("TTSPipeline: not started")
	}
	p.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.textQueue <- textItem{Text: text, Emotion: emotion}:
		atomic.AddInt64(&p.totalEnqueued, 1)
		return nil
	}
}

func (p *ttsPipelineImpl) Interrupt() error {
	p.interruptMu.Lock()
	defer p.interruptMu.Unlock()

	p.mu.Lock()
	if p.interrupting || !p.started {
		p.mu.Unlock()
		return nil
	}
	p.interrupting = true
	p.mu.Unlock()

	logging.Infof("TTSPipeline: interrupting...")

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
			closer.Close()
		}
	}

	p.wg.Wait()

	p.clearQueues()

	p.pendingMu.Lock()
	p.nextSeqNum = 1
	p.nextPlaySeqNum = 1
	p.pendingItems = make(map[int64]*ttsItem)
	p.pendingMu.Unlock()

	p.mu.Lock()
	if p.parentCtx != nil && p.parentCtx.Err() == nil {
		p.ctx, p.cancel = context.WithCancel(p.parentCtx)
		p.startWorkers()
	}
	p.interrupting = false
	atomic.AddInt64(&p.totalInterrupts, 1)
	p.mu.Unlock()

	logging.Infof("TTSPipeline: interrupt completed")
	return nil
}

func (p *ttsPipelineImpl) Stats() PipelineStats {
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

func (p *ttsPipelineImpl) SetSink(sink AudioSink) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sink = sink
}

func (p *ttsPipelineImpl) SetOnPlaybackFinished(callback PlaybackFinishedCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onPlaybackFinished = callback
}

func (p *ttsPipelineImpl) SetOnItemStarted(callback TTSItemStartedCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onItemStarted = callback
}

func (p *ttsPipelineImpl) textConsumer() {
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

func (p *ttsPipelineImpl) ttsWorker(item textItem, seqNum int64) {
	defer p.wg.Done()

	select {
	case <-p.ctx.Done():
		p.notifySeqCompleted(seqNum, nil)
		return
	case p.ttsSemaphore <- struct{}{}:
		defer func() { <-p.ttsSemaphore }()
	}

	streamID := atomic.AddInt64(&p.streamCounter, 1)

	reader, err := p.generateTTS(p.ctx, item.Text, item.Emotion)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			logging.Errorf("TTSPipeline: [stream-%d seq-%d] TTS generation error: %v", streamID, seqNum, err)
		}
		p.notifySeqCompleted(seqNum, nil)
		return
	}

	notifyReader := newEOFNotifyReader(reader)

	ttsItem := &ttsItem{
		Reader:     notifyReader,
		OrigReader: reader,
		Text:       item.Text,
		Emotion:    item.Emotion,
		DoneCh:     make(chan struct{}),
		StreamID:   streamID,
		SeqNum:     seqNum,
	}

	p.notifySeqCompleted(seqNum, ttsItem)
}

func (p *ttsPipelineImpl) notifySeqCompleted(seqNum int64, item *ttsItem) {
	p.pendingMu.Lock()

	if item != nil {
		p.pendingItems[seqNum] = item
	}

	var itemsToSend []*ttsItem
	for {
		nextItem, ok := p.pendingItems[p.nextPlaySeqNum]
		if !ok {
			break
		}
		delete(p.pendingItems, p.nextPlaySeqNum)
		p.nextPlaySeqNum++

		if nextItem != nil {
			itemsToSend = append(itemsToSend, nextItem)
		}
	}
	p.pendingMu.Unlock()

	for _, itm := range itemsToSend {
		select {
		case <-p.ctx.Done():
			itm.Reader.Close()
			if closer, ok := itm.OrigReader.(io.Closer); ok {
				closer.Close()
			}
			return
		case p.ttsBuffer <- itm:
		}
	}
}

func (p *ttsPipelineImpl) audioPlayer() {
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

func (p *ttsPipelineImpl) playItem(item *ttsItem) {
	p.mu.Lock()
	p.currentItem = item
	sink := p.sink
	startedCallback := p.onItemStarted
	p.mu.Unlock()

	if startedCallback != nil {
		startedCallback(item.Text, item.Emotion)
	}

	if sink != nil {
		buf := make([]byte, 4096)
		for {
			select {
			case <-p.ctx.Done():
				item.Reader.Close()
				if closer, ok := item.OrigReader.(io.Closer); ok {
					closer.Close()
				}
				goto cleanup
			default:
			}
			n, err := item.Reader.Read(buf)
			if n > 0 {
				samples := bytesToInt16LE(buf[:n])
				if writeErr := sink.WritePCM(samples); writeErr != nil {
					logging.Errorf("TTSPipeline: sink write failed: %v", writeErr)
					break
				}
			}
			if err != nil {
				break
			}
		}
	}

	if closer, ok := item.OrigReader.(io.Closer); ok {
		closer.Close()
	}

cleanup:
	p.mu.Lock()
	p.currentItem = nil
	p.mu.Unlock()

	atomic.AddInt64(&p.totalPlayed, 1)
	close(item.DoneCh)

	p.mu.Lock()
	finishedCallback := p.onPlaybackFinished
	p.mu.Unlock()
	if finishedCallback != nil {
		finishedCallback()
	}
}

func (p *ttsPipelineImpl) generateTTS(ctx context.Context, text string, emotion string) (io.Reader, error) {
	voice := p.getVoice(emotion)

	cfg := p.ttsConfig
	cfg.Voice = voice

	ttsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	stream, err := p.provider.Start(ttsCtx, cfg)
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

	ttsSampleRate := stream.SampleRate()
	ttsChannels := stream.Channels()
	systemSampleRate := 16000

	var reader io.Reader = audioReader
	if ttsSampleRate != systemSampleRate {
		r := resampler.NewLinearResampler()
		reader = resampler.NewResamplingReader(audioReader, ttsSampleRate, systemSampleRate, ttsChannels, r)
	}

	return reader, nil
}

func (p *ttsPipelineImpl) BeginSession(emotion string) error {
	p.mu.Lock()
	ctx := p.ctx
	if !p.started {
		p.mu.Unlock()
		return errors.New("TTSPipeline: not started")
	}
	p.mu.Unlock()

	voice := p.getVoice(emotion)
	cfg := p.ttsConfig
	cfg.Voice = voice

	stream, err := p.provider.Start(ctx, cfg)
	if err != nil {
		return err
	}

	p.sessionMu.Lock()
	p.activeStream = stream
	p.sessionMu.Unlock()

	audioReader := stream.AudioReader()

	ttsSampleRate := stream.SampleRate()
	ttsChannels := stream.Channels()
	systemSampleRate := 16000
	var reader io.Reader = audioReader
	if ttsSampleRate != systemSampleRate {
		r := resampler.NewLinearResampler()
		reader = resampler.NewResamplingReader(audioReader, ttsSampleRate, systemSampleRate, ttsChannels, r)
	}

	streamID := atomic.AddInt64(&p.streamCounter, 1)
	notifyReader := newEOFNotifyReader(reader)
	item := &ttsItem{
		Reader:     notifyReader,
		OrigReader: audioReader,
		Text:       "[stream]",
		Emotion:    emotion,
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

func (p *ttsPipelineImpl) WriteChunk(chunk string) error {
	p.sessionMu.Lock()
	stream := p.activeStream
	p.sessionMu.Unlock()

	if stream == nil {
		return errors.New("TTSPipeline: no active session")
	}

	p.mu.Lock()
	ctx := p.ctx
	p.mu.Unlock()

	return stream.WriteTextChunk(ctx, chunk)
}

func (p *ttsPipelineImpl) EndSession() error {
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

func (p *ttsPipelineImpl) getVoice(emotion string) string {
	if voice, ok := p.voiceMap[emotion]; ok {
		return voice
	}
	if voice, ok := p.voiceMap["default"]; ok {
		return voice
	}
	return "longanyang"
}

func (p *ttsPipelineImpl) clearQueues() {
	cleared := 0
	for {
		select {
		case <-p.textQueue:
			cleared++
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
				closer.Close()
			}
		}
	}
	p.pendingItems = make(map[int64]*ttsItem)
	p.pendingMu.Unlock()

	cleared = 0
	for {
		select {
		case item := <-p.ttsBuffer:
			item.Reader.Close()
			if closer, ok := item.OrigReader.(io.Closer); ok {
				closer.Close()
			}
			cleared++
		default:
			return
		}
	}
}

func bytesToInt16LE(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return samples
}

func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen])
}
