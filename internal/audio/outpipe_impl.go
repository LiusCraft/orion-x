package audio

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/tts"
)

// outPipeImpl AudioOutPipe 实现
// 集成 TTSPipeline 实现异步 TTS 播放
type outPipeImpl struct {
	pipeline    TTSPipeline
	mixer       AudioMixer
	mixerConfig *MixerConfig
	voiceMap    map[string]string
	ttsConfig   tts.Config
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
}

// NewOutPipe 创建新的 AudioOutPipe（简单版本）
func NewOutPipe(apiKey string) AudioOutPipe {
	cfg := DefaultOutPipeConfig()
	cfg.TTS.APIKey = apiKey
	return NewOutPipeWithConfig(cfg)
}

// NewOutPipeWithConfig 创建新的 AudioOutPipe（带配置）
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

	// 确保 mixer config 存在
	mixerConfig := cfg.Mixer
	if mixerConfig == nil {
		mixerConfig = DefaultMixerConfig()
	}

	// 创建 TTS Pipeline
	provider := tts.NewDashScopeProvider()
	pipelineConfig := cfg.TTSPipeline
	if pipelineConfig == nil {
		pipelineConfig = DefaultTTSPipelineConfig()
	}

	pipeline := NewTTSPipeline(
		provider,
		pipelineConfig,
		cfg.TTS,
		voiceMap,
		mixerConfig,
	)

	return &outPipeImpl{
		pipeline:    pipeline,
		voiceMap:    voiceMap,
		mixerConfig: mixerConfig,
		ttsConfig:   cfg.TTS,
	}
}

func (p *outPipeImpl) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ctx, p.cancel = context.WithCancel(ctx)

	// 启动 TTSPipeline
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

	// 停止 TTSPipeline
	if err := p.pipeline.Stop(); err != nil {
		logging.Errorf("AudioOutPipe: error stopping TTSPipeline: %v", err)
	}

	logging.Infof("AudioOutPipe: stopped")
	return nil
}

func (p *outPipeImpl) SetMixer(mixer AudioMixer) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.mixer = mixer
	p.pipeline.SetMixer(mixer)
}

func (p *outPipeImpl) SetReferenceSink(sink ReferenceSink) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.pipeline.SetReferenceSink(sink)
}

func (p *outPipeImpl) SetOnPlaybackFinished(callback PlaybackFinishedCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.pipeline.SetOnPlaybackFinished(callback)
}

// PlayTTS 播放 TTS（异步，立即返回）
// 文本会被加入队列，由 TTSPipeline 异步处理
func (p *outPipeImpl) PlayTTS(text string, emotion string) error {
	if text == "" {
		return nil
	}

	logging.Infof("AudioOutPipe: PlayTTS (async) - text: %.50s..., emotion: %s",
		truncateForLog(text, 50), emotion)

	// 非阻塞入队
	return p.pipeline.EnqueueText(text, emotion)
}

// PlayResource 播放资源音频
func (p *outPipeImpl) PlayResource(audio io.Reader) error {
	p.mu.Lock()
	mixer := p.mixer
	p.mu.Unlock()

	if mixer == nil {
		return fmt.Errorf("AudioOutPipe: mixer not set")
	}

	logging.Infof("AudioOutPipe: adding resource stream to mixer...")
	mixer.AddResourceStream(audio)
	return nil
}

// Interrupt 中断所有任务（清空队列、停止播放）
func (p *outPipeImpl) Interrupt() error {
	logging.Infof("AudioOutPipe: interrupting...")

	// 委托给 TTSPipeline 处理
	if err := p.pipeline.Interrupt(); err != nil {
		logging.Errorf("AudioOutPipe: interrupt error: %v", err)
		return err
	}

	// 移除资源音频流
	p.mu.Lock()
	mixer := p.mixer
	p.mu.Unlock()

	if mixer != nil {
		mixer.RemoveResourceStream()
	}

	logging.Infof("AudioOutPipe: interrupted")
	return nil
}

// Stats 获取 Pipeline 统计信息
func (p *outPipeImpl) Stats() PipelineStats {
	return p.pipeline.Stats()
}

// truncateForLog 截断文本用于日志显示
func truncateForLog(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen])
}
