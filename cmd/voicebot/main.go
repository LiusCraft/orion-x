package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gordonklaus/portaudio"
	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	audiosink "github.com/liuscraft/orion-x/internal/audio/sink"
	"github.com/liuscraft/orion-x/internal/audio/source"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	_ "github.com/liuscraft/orion-x/internal/provider/llm/register"
	"github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/internal/voicebot"
)

func main() {
	configPath := flag.String("config", config.DefaultPath, "config file path")
	flag.Parse()

	appConfig, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	if err := appConfig.ValidateKeys(true, true, true); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid config: %v\n", err)
		os.Exit(1)
	}
	asrCfg := appConfig.Provider.ASR.Aliyun
	ttsCfg := appConfig.Provider.TTS.Aliyun
	llmCfg := appConfig.Provider.LLM.OpenAI

	if err := logging.Init(logging.Config{
		Level:  appConfig.Logging.Level,
		Format: appConfig.Logging.Format,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logging.Sync()

	logging.SetTraceID(logging.NewTraceID())

	logging.Infof("========================================")
	logging.Infof("        VoiceBot Starting...           ")
	logging.Infof("========================================")

	logging.Infof("Config loaded successfully")
	baseCtx := memory.WithContext(context.Background(), memory.Context{
		UserID:    "local",
		SessionID: "local",
	})

	memCfg := memory.Config{
		Mode:                 memory.Mode(strings.TrimSpace(appConfig.Memory.Mode)),
		SessionMaxTurns:      appConfig.Memory.SessionMaxTurns,
		SessionSummaryEveryN: appConfig.Memory.SessionSummaryEveryN,
		LongTermDBPath:       appConfig.Memory.LongTermDBPath,
		LongTermMaxResults:   appConfig.Memory.LongTermMaxResults,
		RetentionDays:        appConfig.Memory.RetentionDays,
		FTSMinScore:          appConfig.Memory.FTSMinScore,
	}
	memorySvc, err := memory.NewService(memCfg, memory.Options{
		SystemPrompt: agent.DefaultSystemPrompt(),
		LLM: memory.LLMConfig{
			Provider: appConfig.Provider.LLM.Type,
			APIKey:   llmCfg.APIKey,
			BaseURL:  llmCfg.BaseURL,
			Model:    llmCfg.Model,
		},
	})
	if err != nil {
		logging.Fatalf("Failed to create memory service: %v", err)
	}
	defer func() {
		if err := memorySvc.Close(); err != nil {
			logging.Warnf("Close memory service failed: %v", err)
		}
	}()
	toolCfg := tools.ManagerConfig{
		Provider:        appConfig.Provider.LLM.Type,
		APIKey:          llmCfg.APIKey,
		BaseURL:         llmCfg.BaseURL,
		Model:           llmCfg.Model,
		ToolTypes:       appConfig.Tools.Types,
		ActionResponses: appConfig.Tools.ActionResponses,
		MCPServers:      toToolsMCPServers(appConfig.Tools.MCP),
	}

	logging.Infof("Creating ToolManager...")
	toolManager, err := tools.NewToolManager(baseCtx, toolCfg)
	if err != nil {
		logging.Fatalf("Failed to create ToolManager: %v", err)
	}
	defer func() {
		if err := toolManager.Close(); err != nil {
			logging.Warnf("Close ToolManager failed: %v", err)
		}
	}()
	logging.Infof("ToolManager created successfully")

	logging.Infof("Creating VoiceAgent...")
	voiceAgent, err := agent.NewVoiceAgentWithToolManagerAndMemory(baseCtx, toolCfg, toolManager, memorySvc)
	if err != nil {
		logging.Fatalf("Failed to create VoiceAgent: %v", err)
	}
	logging.Infof("VoiceAgent created successfully")

	logging.Infof("Creating AudioMixer...")
	mixerCfg := &audio.MixerConfig{
		TTSVolume:       appConfig.Audio.Mixer.TTSVolume,
		ResourceVolume:  appConfig.Audio.Mixer.ResourceVolume,
		SampleRate:      appConfig.Audio.Mixer.SampleRate,
		Channels:        appConfig.Audio.Mixer.Channels,
		FramesPerBuffer: appConfig.Audio.Mixer.FramesPerBuffer,
	}
	// Initialize PortAudio once for all audio components
	logging.Infof("Initializing PortAudio...")
	if err := portaudio.Initialize(); err != nil {
		logging.Fatalf("Failed to initialize PortAudio: %v", err)
	}
	defer portaudio.Terminate()
	logging.Infof("PortAudio initialized successfully")

	logging.Infof("Creating AudioMixer...")
	mixer, err := audio.NewMixer(mixerCfg)
	if err != nil {
		logging.Fatalf("Failed to create AudioMixer: %v", err)
	}
	logging.Infof("AudioMixer created successfully")

	logging.Infof("Starting AudioMixer...")
	mixerSink := audiosink.NewPortAudioSink()
	mixer.SetSink(mixerSink)
	if err := mixer.Start(); err != nil {
		logging.Fatalf("Failed to start AudioMixer: %v", err)
	}
	logging.Infof("AudioMixer started")

	logging.Infof("Creating AudioOutPipe...")
	outPipeCfg := audio.DefaultOutPipeConfig()
	outPipeCfg.Mixer = mixerCfg
	outPipeCfg.TTSProviderType = appConfig.Provider.TTS.Type
	// 配置 TTS Pipeline
	outPipeCfg.TTSPipeline = &audio.TTSPipelineConfig{
		MaxTTSBuffer:     appConfig.Audio.TTSPipeline.MaxTTSBuffer,
		MaxConcurrentTTS: appConfig.Audio.TTSPipeline.MaxConcurrentTTS,
		TextQueueSize:    appConfig.Audio.TTSPipeline.TextQueueSize,
	}
	// 如果配置值为 0，使用默认值
	if outPipeCfg.TTSPipeline.MaxTTSBuffer <= 0 {
		outPipeCfg.TTSPipeline.MaxTTSBuffer = 3
	}
	if outPipeCfg.TTSPipeline.MaxConcurrentTTS <= 0 {
		outPipeCfg.TTSPipeline.MaxConcurrentTTS = 2
	}
	if outPipeCfg.TTSPipeline.TextQueueSize <= 0 {
		outPipeCfg.TTSPipeline.TextQueueSize = 100
	}
	outPipeCfg.TTS = tts.Config{
		APIKey:               ttsCfg.APIKey,
		Endpoint:             ttsCfg.Endpoint,
		Workspace:            ttsCfg.Workspace,
		Model:                ttsCfg.Model,
		Voice:                ttsCfg.Voice,
		Format:               ttsCfg.Format,
		SampleRate:           ttsCfg.SampleRate,
		Volume:               ttsCfg.Volume,
		Rate:                 ttsCfg.Rate,
		Pitch:                ttsCfg.Pitch,
		EnableSSML:           ttsCfg.EnableSSML,
		TextType:             ttsCfg.TextType,
		EnableDataInspection: ttsCfg.EnableDataInspection,
	}
	if len(ttsCfg.VoiceMap) > 0 {
		outPipeCfg.VoiceMap = ttsCfg.VoiceMap
	}
	audioOutPipe := audio.NewOutPipeWithConfig(outPipeCfg)
	audioOutPipe.SetMixer(mixer)
	logging.Infof("AudioOutPipe created successfully (async TTS pipeline: maxBuffer=%d, maxConcurrent=%d)",
		outPipeCfg.TTSPipeline.MaxTTSBuffer, outPipeCfg.TTSPipeline.MaxConcurrentTTS)

	logging.Infof("Creating AudioInPipe...")
	inPipeCfg := &audio.InPipeConfig{
		SampleRate:      appConfig.Audio.InPipe.SampleRate,
		Channels:        appConfig.Audio.InPipe.Channels,
		EnableVAD:       appConfig.Audio.InPipe.EnableVAD,
		VADThreshold:    appConfig.Audio.InPipe.VADThreshold,
		VADType:         appConfig.Audio.InPipe.VADType,
		VADModelPath:    appConfig.Audio.InPipe.VADModelPath,
		VADMinSilenceMs: appConfig.Audio.InPipe.VADMinSilenceMs,
		VADSpeechPadMs:  appConfig.Audio.InPipe.VADSpeechPadMs,
		ASRProviderType: appConfig.Provider.ASR.Type,
		ASRModel:        asrCfg.Model,
		ASREndpoint:     asrCfg.Endpoint,
	}

	// 配置缓冲区大小，默认 3200 样本 (200ms @ 16kHz)
	bufferSize := appConfig.Audio.InPipe.BufferSize
	if bufferSize <= 0 {
		bufferSize = 3200
	}

	logging.Infof("Creating Microphone source (bufferSize=%d, highLatency=%v, inputDevice=%q)...",
		bufferSize, appConfig.Audio.InPipe.HighLatency, appConfig.Audio.InPipe.InputDevice)
	micSource, err := source.NewMicrophoneSourceWithDevice(
		inPipeCfg.SampleRate,
		inPipeCfg.Channels,
		bufferSize,
		appConfig.Audio.InPipe.HighLatency,
		appConfig.Audio.InPipe.InputDevice,
	)
	if err != nil {
		logging.Fatalf("Failed to create Microphone source: %v", err)
	}
	logging.Infof("Microphone source created successfully")

	audioSource := audio.AudioSource(micSource)

	audioInPipe, err := audio.NewInPipeWithAudioSource(asrCfg.APIKey, inPipeCfg, audioSource)
	if err != nil {
		logging.Fatalf("Failed to create AudioInPipe: %v", err)
	}
	logging.Infof("AudioInPipe created successfully")

	// 创建工具执行器适配器
	toolExecutor := tools.NewExecutorAdapter(baseCtx, toolManager)

	logging.Infof("Creating Orchestrator...")
	orchestrator := voicebot.NewOrchestratorWithOptions(
		voiceAgent,
		audioOutPipe,
		audioInPipe,
		toolExecutor,
		&voicebot.OrchestratorOptions{Memory: memorySvc},
	)
	logging.Infof("Orchestrator created successfully")

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	logging.Infof("Setting up signal handler...")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logging.Infof("\n========================================")
		logging.Infof("     Received interrupt signal...       ")
		logging.Infof("========================================")

		// 关闭顺序：从外到内，先停止依赖方，再停止被依赖方
		// Orchestrator 依赖 Mixer，所以先停 Orchestrator
		logging.Infof("Stopping Orchestrator...")
		if err := orchestrator.Stop(); err != nil {
			logging.Errorf("Error stopping orchestrator: %v", err)
		}

		logging.Infof("Stopping Mixer...")
		if err := mixer.Stop(); err != nil {
			logging.Errorf("Error stopping mixer: %v", err)
		}

		// 取消 context，让 main 函数自然退出
		// 不使用 os.Exit(0)，这样 defer 语句（如 portaudio.Terminate()）才会被执行
		cancel()
	}()

	logging.Infof("Starting Orchestrator...")
	if err := orchestrator.Start(ctx); err != nil {
		logging.Fatalf("Failed to start orchestrator: %v", err)
	}

	logging.Infof("========================================")
	logging.Infof("     VoiceBot is Running! 🎤          ")
	logging.Infof("     Press Ctrl+C to stop.             ")
	logging.Infof("========================================")

	// Wait for context cancellation (triggered by signal handler)
	<-ctx.Done()

	logging.Infof("\n========================================")
	logging.Infof("     VoiceBot Shutting Down...          ")
	logging.Infof("========================================")

	// PortAudio 会在 defer portaudio.Terminate() 中被清理
	logging.Infof("VoiceBot stopped.")
}

func toToolsMCPServers(cfgs []config.MCPServerConfig) []tools.MCPServerConfig {
	servers := make([]tools.MCPServerConfig, 0, len(cfgs))
	for _, cfg := range cfgs {
		servers = append(servers, tools.MCPServerConfig{
			ID:           cfg.ID,
			Transport:    cfg.Transport,
			Command:      cfg.Command,
			Args:         cfg.Args,
			Endpoint:     cfg.Endpoint,
			ToolNameList: cfg.ToolNameList,
			TimeoutMs:    cfg.TimeoutMs,
		})
	}
	return servers
}
