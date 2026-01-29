package audio

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/tts"
)

// eofNotifyReader wraps an io.Reader and signals when EOF is reached
// This allows TTSPipeline to wait for Mixer to finish reading without consuming data itself
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
		// Signal done on any error (EOF or otherwise)
		r.once.Do(func() { close(r.doneCh) })
	}
	return n, err
}

// Done returns a channel that is closed when the reader reaches EOF or error
func (r *eofNotifyReader) Done() <-chan struct{} {
	return r.doneCh
}

// Close signals done if not already done (for cleanup on interrupt)
func (r *eofNotifyReader) Close() {
	r.once.Do(func() { close(r.doneCh) })
}

// ttsItem TTS 缓冲区项
type ttsItem struct {
	Reader     *eofNotifyReader // 带 EOF 通知的 reader
	OrigReader io.Reader        // 原始 reader（用于关闭）
	Emotion    string
	DoneCh     chan struct{} // 播放完成信号
	StreamID   int64         // 用于追踪
	SeqNum     int64         // 序号，用于保证播放顺序
}

// ttsPipelineImpl TTSPipeline 实现
type ttsPipelineImpl struct {
	config      *TTSPipelineConfig
	provider    tts.Provider
	ttsConfig   tts.Config
	voiceMap    map[string]string
	mixerConfig *MixerConfig

	// 外部依赖（可动态设置）
	mixer              AudioMixer
	reference          ReferenceSink
	onPlaybackFinished PlaybackFinishedCallback

	// 队列
	textQueue chan textItem
	ttsBuffer chan *ttsItem

	// 并发控制
	ttsSemaphore chan struct{}

	// 顺序控制：保证 TTS 按入队顺序播放
	nextSeqNum     int64              // 下一个要分配的序号
	nextPlaySeqNum int64              // 下一个要播放的序号
	pendingItems   map[int64]*ttsItem // 已完成但等待播放的 TTS 项
	pendingMu      sync.Mutex         // 保护 pendingItems

	// 状态
	currentItem   *ttsItem
	parentCtx     context.Context
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.Mutex
	interruptMu   sync.Mutex // 防止并发 Interrupt 调用
	started       bool
	interrupting  bool
	streamCounter int64

	// 统计
	totalEnqueued   int64
	totalPlayed     int64
	totalInterrupts int64
}

// NewTTSPipeline 创建新的 TTS Pipeline
func NewTTSPipeline(
	provider tts.Provider,
	config *TTSPipelineConfig,
	ttsConfig tts.Config,
	voiceMap map[string]string,
	mixerConfig *MixerConfig,
) TTSPipeline {
	if config == nil {
		config = DefaultTTSPipelineConfig()
	}
	if voiceMap == nil {
		voiceMap = map[string]string{
			"default": "longanyang",
		}
	}
	if mixerConfig == nil {
		mixerConfig = DefaultMixerConfig()
	}

	return &ttsPipelineImpl{
		config:         config,
		provider:       provider,
		ttsConfig:      ttsConfig,
		voiceMap:       voiceMap,
		mixerConfig:    mixerConfig,
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
	// Text Consumer - 从文本队列取出，启动 TTS Worker
	p.wg.Add(1)
	go p.textConsumer()

	// Audio Player - 从 TTS 缓冲区取出，播放
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

	// 主动关闭当前正在播放的 item 的 reader，解除 playItem 中的阻塞
	currentItem := p.currentItem
	p.mu.Unlock()

	if currentItem != nil {
		// 关闭 eofNotifyReader，通知 playItem 退出
		currentItem.Reader.Close()
		// 关闭原始 reader（bufferedPipe），解除 Mixer 的读取阻塞
		if closer, ok := currentItem.OrigReader.(io.Closer); ok {
			closer.Close()
		}
	}

	// 启动一个 goroutine 持续清空 ttsBuffer，直到被通知停止
	// 这样 ttsWorker 中的 notifySeqCompleted 才不会因为 buffer 满而阻塞
	stopDrainer := make(chan struct{})
	drainerDone := make(chan struct{})
	go func() {
		defer close(drainerDone)
		drained := 0
		for {
			select {
			case <-stopDrainer:
				// 被通知停止，做最后一次清理
				for {
					select {
					case item := <-p.ttsBuffer:
						item.Reader.Close()
						if closer, ok := item.OrigReader.(io.Closer); ok {
							closer.Close()
						}
						drained++
					default:
						return
					}
				}
			case item := <-p.ttsBuffer:
				// 消费 buffer 中的 item
				item.Reader.Close()
				if closer, ok := item.OrigReader.(io.Closer); ok {
					closer.Close()
				}
				drained++
			}
		}
	}()

	// 等待所有 worker 退出
	p.wg.Wait()

	// 停止 drainer goroutine
	close(stopDrainer)
	<-drainerDone

	// 清空队列
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
	// 使用独立的互斥锁防止并发 Interrupt 调用
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

	p.mu.Lock()
	// 1. 取消当前 context（通知所有 worker 停止）
	if p.cancel != nil {
		p.cancel()
	}

	// 2. 立即停止当前播放
	currentItem := p.currentItem
	if currentItem != nil {
		if p.mixer != nil {
			p.mixer.RemoveTTSStream()
			p.mixer.OnTTSFinished()
		}
		p.currentItem = nil
	}
	p.mu.Unlock()

	// 3. 关闭当前正在播放的 item 的 reader，解除 playItem 中的阻塞
	if currentItem != nil {
		currentItem.Reader.Close()
		if closer, ok := currentItem.OrigReader.(io.Closer); ok {
			closer.Close()
		}
	}

	// 4. 等待所有 worker 退出
	p.wg.Wait()

	// 5. 清空队列
	p.clearQueues()

	// 6. 重置序号计数器
	p.pendingMu.Lock()
	p.nextSeqNum = 1
	p.nextPlaySeqNum = 1
	p.pendingItems = make(map[int64]*ttsItem)
	p.pendingMu.Unlock()

	// 7. 重新创建 context 和 workers
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

func (p *ttsPipelineImpl) SetMixer(mixer AudioMixer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mixer = mixer
}

func (p *ttsPipelineImpl) SetReferenceSink(sink ReferenceSink) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reference = sink
}

func (p *ttsPipelineImpl) SetOnPlaybackFinished(callback PlaybackFinishedCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onPlaybackFinished = callback
}

// textConsumer 文本消费者 goroutine
// 从 textQueue 取出文本，分配序号，启动 TTS Worker 生成音频
func (p *ttsPipelineImpl) textConsumer() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case item := <-p.textQueue:
			// 分配序号（保证顺序）
			p.pendingMu.Lock()
			seqNum := p.nextSeqNum
			p.nextSeqNum++
			p.pendingMu.Unlock()

			// 启动 TTS Worker（受 semaphore 限制）
			p.wg.Add(1)
			go p.ttsWorker(item, seqNum)
		}
	}
}

// ttsWorker TTS 生成 worker
// 生成 TTS 音频流，通过 pendingItems 保证顺序
func (p *ttsPipelineImpl) ttsWorker(item textItem, seqNum int64) {
	defer p.wg.Done()

	// 获取 semaphore（限制并发数）
	select {
	case <-p.ctx.Done():
		// 被取消，需要通知可能在等待的 audioPlayer
		p.notifySeqCompleted(seqNum, nil)
		return
	case p.ttsSemaphore <- struct{}{}:
		defer func() { <-p.ttsSemaphore }()
	}

	streamID := atomic.AddInt64(&p.streamCounter, 1)

	// 生成 TTS
	reader, err := p.generateTTS(p.ctx, item.Text, item.Emotion)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			logging.Errorf("TTSPipeline: [stream-%d seq-%d] TTS generation error: %v", streamID, seqNum, err)
		}
		// 通知序号完成（即使失败），让后续序号可以继续
		p.notifySeqCompleted(seqNum, nil)
		return
	}

	// 创建带 EOF 通知的 reader
	notifyReader := newEOFNotifyReader(reader)

	// 创建 ttsItem
	ttsItem := &ttsItem{
		Reader:     notifyReader,
		OrigReader: reader,
		Emotion:    item.Emotion,
		DoneCh:     make(chan struct{}),
		StreamID:   streamID,
		SeqNum:     seqNum,
	}

	// 通知序号完成，放入 pending 等待按序播放
	p.notifySeqCompleted(seqNum, ttsItem)
}

// notifySeqCompleted 通知某个序号的 TTS 已完成（成功或失败）
func (p *ttsPipelineImpl) notifySeqCompleted(seqNum int64, item *ttsItem) {
	p.pendingMu.Lock()

	if item != nil {
		p.pendingItems[seqNum] = item
	}

	// 收集需要按顺序发送的 items
	var itemsToSend []*ttsItem
	for {
		nextItem, ok := p.pendingItems[p.nextPlaySeqNum]
		if !ok {
			// 下一个序号还没完成，等待
			break
		}
		delete(p.pendingItems, p.nextPlaySeqNum)
		p.nextPlaySeqNum++

		if nextItem != nil {
			itemsToSend = append(itemsToSend, nextItem)
		}
	}
	p.pendingMu.Unlock()

	// 在锁外按顺序同步发送到 ttsBuffer
	for _, itm := range itemsToSend {
		select {
		case <-p.ctx.Done():
			// 被取消，关闭 reader
			itm.Reader.Close()
			if closer, ok := itm.OrigReader.(io.Closer); ok {
				closer.Close()
			}
			return
		case p.ttsBuffer <- itm:
			// 成功入队
		}
	}
}

// audioPlayer 音频播放器 goroutine
// 从 ttsBuffer 取出 TTS 流，播放
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

// playItem 播放单个 TTS 流
func (p *ttsPipelineImpl) playItem(item *ttsItem) {
	p.mu.Lock()
	p.currentItem = item
	mixer := p.mixer
	p.mu.Unlock()

	if mixer != nil {
		mixer.OnTTSStarted()
		// 将 eofNotifyReader 传给 Mixer，Mixer 读取时会触发 EOF 通知
		mixer.AddTTSStream(item.Reader)
	}

	// 等待播放完成：Mixer 读取到 EOF 时，item.Reader.Done() 会被关闭
	select {
	case <-p.ctx.Done():
		// 被打断，确保通知 reader done
		item.Reader.Close()
		// 同时关闭原始 reader，解除可能的读取阻塞
		if closer, ok := item.OrigReader.(io.Closer); ok {
			closer.Close()
		}
	case <-item.Reader.Done():
		// Mixer 读取完毕
	}

	// 播放完成
	p.mu.Lock()
	p.currentItem = nil
	p.mu.Unlock()

	if mixer != nil {
		mixer.OnTTSFinished()
		mixer.RemoveTTSStream()
	}

	// 关闭原始 reader（如果支持）
	if closer, ok := item.OrigReader.(io.Closer); ok {
		closer.Close()
	}

	atomic.AddInt64(&p.totalPlayed, 1)
	close(item.DoneCh)

	// 通知播放完成
	p.mu.Lock()
	callback := p.onPlaybackFinished
	p.mu.Unlock()
	if callback != nil {
		callback()
	}
}

// generateTTS 生成 TTS 音频流
func (p *ttsPipelineImpl) generateTTS(ctx context.Context, text string, emotion string) (io.Reader, error) {
	voice := p.getVoice(emotion)

	cfg := p.ttsConfig
	cfg.Voice = voice

	// 创建带超时的 context
	ttsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 启动 TTS 流
	stream, err := p.provider.Start(ttsCtx, cfg)
	if err != nil {
		return nil, err
	}

	// 写入文本
	if err := stream.WriteTextChunk(ttsCtx, text); err != nil {
		stream.Close(ttsCtx)
		return nil, err
	}

	// 关闭写入（通知 TTS 服务文本发送完毕）
	if err := stream.Close(ttsCtx); err != nil {
		return nil, err
	}

	// 获取音频 reader
	audioReader := stream.AudioReader()

	// 检测采样率并进行重采样
	ttsSampleRate := stream.SampleRate()
	ttsChannels := stream.Channels()
	systemSampleRate := 16000
	if p.mixerConfig != nil && p.mixerConfig.SampleRate > 0 {
		systemSampleRate = p.mixerConfig.SampleRate
	}

	var reader io.Reader = audioReader
	if ttsSampleRate != systemSampleRate {
		resampler := NewLinearResampler()
		reader = NewResamplingReader(audioReader, ttsSampleRate, systemSampleRate, ttsChannels, resampler)
	}

	// 添加 reference sink（用于 AEC）
	p.mu.Lock()
	reference := p.reference
	p.mu.Unlock()

	if reference != nil {
		reader = &referenceTeeReader{reader: reader, sink: reference}
	}

	return reader, nil
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
	// 清空 textQueue
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
	// 清空 pendingItems
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

	// 清空 ttsBuffer，关闭所有未播放的 reader
	cleared = 0
	for {
		select {
		case item := <-p.ttsBuffer:
			// 关闭 eofNotifyReader 的通知
			item.Reader.Close()
			// 关闭原始 reader
			if closer, ok := item.OrigReader.(io.Closer); ok {
				closer.Close()
			}
			cleared++
		default:
			return
		}
	}
}

// truncateText 截断文本用于日志显示
func truncateText(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen])
}

// referenceTeeReader 将读取的数据同时写入 reference sink
type referenceTeeReader struct {
	reader io.Reader
	sink   ReferenceSink
}

func (r *referenceTeeReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 && r.sink != nil {
		r.sink.WriteReference(p[:n])
	}
	return n, err
}
