package audio

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/provider/tts"
	_ "github.com/liuscraft/orion-x/internal/provider/tts/register"
)

type outPipeImpl struct {
	pipeline   TTSPipeline
	sink       AudioSink
	sinkFormat *AudioFormat
	voiceMap   map[string]string
	ttsConfig  tts.Config
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
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

	ttsProvider := cfg.TTSProvider
	if ttsProvider == nil {
		created, err := tts.NewProvider(tts.ProviderConfig{Type: cfg.TTSProviderType})
		if err != nil {
			logging.Errorf("AudioOutPipe: failed to create TTS provider: %v", err)
			created, _ = tts.NewProvider(tts.ProviderConfig{})
		}
		ttsProvider = created
	}
	pipelineConfig := cfg.TTSPipeline
	if pipelineConfig == nil {
		pipelineConfig = DefaultTTSPipelineConfig()
	}

	pipeline := NewTTSPipeline(
		ttsProvider,
		pipelineConfig,
		cfg.TTS,
		voiceMap,
	)

	sinkFormat := cfg.SinkFormat
	if sinkFormat == nil {
		sinkFormat = &AudioFormat{
			SampleRate:      cfg.TTS.SampleRate,
			Channels:        1,
			FramesPerBuffer: 1024,
		}
		if sinkFormat.SampleRate <= 0 {
			sinkFormat.SampleRate = 16000
		}
	}

	return &outPipeImpl{
		pipeline:   pipeline,
		sinkFormat: sinkFormat,
		voiceMap:   voiceMap,
		ttsConfig:  cfg.TTS,
	}
}

func (p *outPipeImpl) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx, p.cancel = context.WithCancel(ctx)

	if p.sink != nil && p.sinkFormat != nil {
		if err := p.sink.Start(p.ctx, *p.sinkFormat); err != nil {
			return fmt.Errorf("AudioOutPipe: failed to start sink: %w", err)
		}
	}

	if err := p.pipeline.Start(p.ctx); err != nil {
		return fmt.Errorf("AudioOutPipe: failed to start TTSPipeline: %w", err)
	}

	logging.Infof("AudioOutPipe: started (async mode with TTSPipeline)")
	return nil
}

func (p *outPipeImpl) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	logging.Infof("AudioOutPipe: stopping...")

	if p.cancel != nil {
		p.cancel()
	}

	if err := p.pipeline.Stop(); err != nil {
		logging.Errorf("AudioOutPipe: error stopping TTSPipeline: %v", err)
	}

	if p.sink != nil {
		if err := p.sink.Stop(); err != nil {
			logging.Errorf("AudioOutPipe: error stopping sink: %v", err)
		}
	}

	logging.Infof("AudioOutPipe: stopped")
	return nil
}

func (p *outPipeImpl) SetSink(sink AudioSink) {
	p.mu.Lock()
	defer p.mu.Unlock()
	logging.Infof("AudioOutPipe: setting sink: %v", sink)
	p.sink = sink
	if p.pipeline != nil {
		p.pipeline.SetSink(sink)
	}
}

func (p *outPipeImpl) SetOnPlaybackFinished(callback PlaybackFinishedCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pipeline.SetOnPlaybackFinished(callback)
}

func (p *outPipeImpl) SetOnTTSItemStarted(callback TTSItemStartedCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pipeline.SetOnItemStarted(callback)
}

func (p *outPipeImpl) PlayTTS(text string, emotion string) error {
	if text == "" {
		return nil
	}

	logging.Infof("AudioOutPipe: PlayTTS (async) - text: %.50s..., emotion: %s",
		truncateForLog(text, 50), emotion)

	return p.pipeline.EnqueueText(text, emotion)
}

func (p *outPipeImpl) BeginTTSStream(emotion string) error {
	return p.pipeline.BeginSession(emotion)
}

func (p *outPipeImpl) WriteTTSChunk(chunk string) error {
	return p.pipeline.WriteChunk(chunk)
}

func (p *outPipeImpl) EndTTSStream() error {
	return p.pipeline.EndSession()
}

func (p *outPipeImpl) PlayResource(audio io.Reader) error {
	p.mu.Lock()
	sink := p.sink
	p.mu.Unlock()

	if sink == nil {
		return fmt.Errorf("AudioOutPipe: sink not set")
	}

	buf := make([]byte, 4096)
	for {
		n, err := audio.Read(buf)
		if n > 0 {
			samples := bytesToInt16LE(buf[:n])
			if writeErr := sink.WritePCM(samples); writeErr != nil {
				return fmt.Errorf("AudioOutPipe: sink write error: %w", writeErr)
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("AudioOutPipe: resource read error: %w", err)
		}
	}
}

func (p *outPipeImpl) Interrupt() error {
	logging.Infof("AudioOutPipe: interrupting...")

	if err := p.pipeline.Interrupt(); err != nil {
		logging.Errorf("AudioOutPipe: interrupt error: %v", err)
		return err
	}

	logging.Infof("AudioOutPipe: interrupted")
	return nil
}

func (p *outPipeImpl) Stats() PipelineStats {
	return p.pipeline.Stats()
}

func truncateForLog(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen])
}
