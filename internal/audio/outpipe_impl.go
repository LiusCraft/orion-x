package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/tts"
)

type outPipeImpl struct {
	mixer       AudioMixer
	mixerConfig *MixerConfig
	tts         tts.Provider
	ttsStreams  []tts.Stream
	voiceMap    map[string]string
	ttsConfig   tts.Config
	apiKey      string
	reference   ReferenceSink
	ctx         context.Context
	cancel      context.CancelFunc
	ttsCancel   context.CancelFunc // 用于取消当前正在进行的 TTS
	mu          sync.Mutex
}

type ttsStreamReader struct {
	reader   io.Reader
	doneOnce sync.Once
	onDone   func()
}

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

func (r *ttsStreamReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err != nil {
		r.done()
	}
	return n, err
}

func (r *ttsStreamReader) done() {
	r.doneOnce.Do(func() {
		if r.onDone != nil {
			r.onDone()
		}
	})
}

func NewOutPipe(apiKey string) AudioOutPipe {
	cfg := DefaultOutPipeConfig()
	cfg.TTS.APIKey = apiKey
	return NewOutPipeWithConfig(cfg)
}

func NewOutPipeWithConfig(cfg *OutPipeConfig) AudioOutPipe {
	if cfg == nil {
		cfg = DefaultOutPipeConfig()
	}
	if len(cfg.VoiceMap) == 0 {
		cfg.VoiceMap = DefaultOutPipeConfig().VoiceMap
	}

	voiceMap := make(map[string]string)
	for key, value := range cfg.VoiceMap {
		voiceMap[key] = value
	}

	return &outPipeImpl{
		voiceMap:    voiceMap,
		mixerConfig: cfg.Mixer,
		ttsConfig:   cfg.TTS,
		apiKey:      cfg.TTS.APIKey,
		tts:         tts.NewDashScopeProvider(),
	}
}

func (p *outPipeImpl) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ctx, p.cancel = context.WithCancel(ctx)
	logging.Infof("AudioOutPipe: started")
	return nil
}

func (p *outPipeImpl) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel != nil {
		p.cancel()
	}

	for _, stream := range p.ttsStreams {
		_ = stream.Close(p.ctx)
	}
	p.ttsStreams = nil

	logging.Infof("AudioOutPipe: stopped")
	return nil
}

func (p *outPipeImpl) SetMixer(mixer AudioMixer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mixer = mixer
}

func (p *outPipeImpl) SetReferenceSink(sink ReferenceSink) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reference = sink
}

func (p *outPipeImpl) PlayTTS(text string, emotion string) error {
	if text == "" {
		return nil
	}

	voice := p.getVoice(emotion)
	logging.Infof("AudioOutPipe: PlayTTS - text: %s, emotion: %s, voice: %s", text, emotion, voice)

	p.mu.Lock()
	ctx := p.ctx
	mixer := p.mixer
	sink := p.reference

	// 创建可被 Interrupt 取消的 context
	if ctx == nil {
		ctx = context.Background()
	}
	ttsCtx, ttsCancel := context.WithTimeout(ctx, 10*time.Second)
	p.ttsCancel = ttsCancel
	p.mu.Unlock()

	defer func() {
		ttsCancel()
		p.mu.Lock()
		p.ttsCancel = nil
		p.mu.Unlock()
	}()

	cfg := p.ttsConfig
	if cfg.APIKey == "" {
		cfg.APIKey = p.apiKey
	}
	if cfg.APIKey == "" {
		return errors.New("tts api key is required")
	}
	cfg.Voice = voice

	logging.Infof("AudioOutPipe: starting TTS stream...")
	stream, err := p.tts.Start(ttsCtx, cfg)
	if err != nil {
		logging.Errorf("AudioOutPipe: TTS start error: %v (ctx=%v)", err, ttsCtx.Err())
		if isRetryableTTSError(err) {
			logging.Warnf("AudioOutPipe: retrying TTS start...")
			time.Sleep(300 * time.Millisecond)
			stream, err = p.tts.Start(ttsCtx, cfg)
		}
		if err != nil {
			logging.Errorf("AudioOutPipe: TTS start retry failed: %v (ctx=%v)", err, ttsCtx.Err())
			return fmt.Errorf("TTS start error: %w", err)
		}
	}

	p.mu.Lock()
	p.ttsStreams = append(p.ttsStreams, stream)
	mixerConfig := p.mixerConfig
	p.mu.Unlock()

	audioReader := stream.AudioReader()
	reader := io.Reader(audioReader)

	// 检测采样率并进行重采样
	ttsSampleRate := stream.SampleRate()
	ttsChannels := stream.Channels()
	systemSampleRate := 16000 // 默认系统采样率
	if mixerConfig != nil && mixerConfig.SampleRate > 0 {
		systemSampleRate = mixerConfig.SampleRate
	}

	if ttsSampleRate != systemSampleRate {
		logging.Infof("AudioOutPipe: TTS sample rate (%d Hz) differs from system (%d Hz), resampling required",
			ttsSampleRate, systemSampleRate)
		resampler := NewLinearResampler()
		reader = NewResamplingReader(reader, ttsSampleRate, systemSampleRate, ttsChannels, resampler)
	} else {
		logging.Debugf("AudioOutPipe: TTS sample rate matches system (%d Hz), no resampling needed", ttsSampleRate)
	}

	if sink != nil {
		reader = &referenceTeeReader{reader: reader, sink: sink}
	}
	wrappedReader := &ttsStreamReader{reader: reader}
	wrappedReader.onDone = func() {
		if mixer != nil {
			mixer.OnTTSFinished()
			mixer.RemoveTTSStream()
		}
		p.removeStream(stream)
	}

	if mixer != nil {
		logging.Infof("AudioOutPipe: adding TTS stream to mixer...")
		mixer.OnTTSStarted()
		mixer.AddTTSStream(wrappedReader)
	} else {
		go p.drainAudio(wrappedReader)
	}

	logging.Infof("AudioOutPipe: writing text chunk to TTS...")
	if err := stream.WriteTextChunk(ttsCtx, text); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logging.Infof("AudioOutPipe: TTS write cancelled due to context cancellation (normal interruption)")
		} else {
			logging.Errorf("AudioOutPipe: TTS write error: %v", err)
		}
		wrappedReader.done()
		return fmt.Errorf("TTS write error: %w", err)
	}

	logging.Infof("AudioOutPipe: closing TTS stream...")
	if err := stream.Close(ttsCtx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			logging.Infof("AudioOutPipe: TTS stream closed due to context cancellation (normal interruption)")
		} else {
			logging.Errorf("AudioOutPipe: TTS close error: %v", err)
		}
		wrappedReader.done()
		return fmt.Errorf("TTS close error: %w", err)
	}

	logging.Infof("AudioOutPipe: PlayTTS completed")
	return nil
}

func (p *outPipeImpl) PlayResource(audio io.Reader) error {
	if p.mixer == nil {
		return fmt.Errorf("mixer not set")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	logging.Infof("AudioOutPipe: adding resource stream to mixer...")
	p.mixer.AddResourceStream(audio)
	return nil
}

func (p *outPipeImpl) Interrupt() error {
	p.mu.Lock()
	ttsCancel := p.ttsCancel
	mixer := p.mixer
	ctx := p.ctx

	// 立即取消正在进行的 TTS
	if ttsCancel != nil {
		ttsCancel()
		p.ttsCancel = nil
	}

	// 关闭所有 TTS streams
	for _, stream := range p.ttsStreams {
		_ = stream.Close(ctx)
	}
	p.ttsStreams = nil
	p.mu.Unlock()

	// 立即从 Mixer 移除音频流，停止播放
	// 注意：不调用 OnTTSFinished，因为 wrappedReader.onDone 会在 reader 返回错误时自动调用
	if mixer != nil {
		mixer.RemoveTTSStream()
		mixer.RemoveResourceStream()
	}

	logging.Infof("AudioOutPipe: interrupted")
	return nil
}

func (p *outPipeImpl) removeStream(target tts.Stream) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.ttsStreams) == 0 {
		return
	}

	for i, stream := range p.ttsStreams {
		if stream == target {
			p.ttsStreams = append(p.ttsStreams[:i], p.ttsStreams[i+1:]...)
			return
		}
	}
}

func (p *outPipeImpl) drainAudio(reader io.Reader) {
	buf := make([]byte, 4096)
	for {
		if _, err := reader.Read(buf); err != nil {
			return
		}
	}
}

func isRetryableTTSError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}

func (p *outPipeImpl) getVoice(emotion string) string {
	if voice, ok := p.voiceMap[emotion]; ok {
		return voice
	}
	return p.voiceMap["default"]
}
