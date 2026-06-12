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
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/pipeline"
	pstages "github.com/liuscraft/orion-x/internal/pipeline/stages"
	"github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
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
	agentCfg := agent.Config{
		Provider:    appConfig.Provider.LLM.Type,
		APIKey:      llmCfg.APIKey,
		BaseURL:     llmCfg.BaseURL,
		Model:       llmCfg.Model,
		ExtraFields: llmCfg.ExtraFields,
	}
	toolCfg := tools.ManagerConfig{
		MCPServers: toToolsMCPServers(appConfig.Tools.MCP),
	}

	logging.Infof("Creating ToolManager...")
	toolMgr, err := tools.NewManager(baseCtx, toolCfg)
	if err != nil {
		logging.Fatalf("Failed to create ToolManager: %v", err)
	}
	defer func() {
		if err := toolMgr.Close(); err != nil {
			logging.Warnf("Close ToolManager failed: %v", err)
		}
	}()
	logging.Infof("ToolManager created successfully")

	logging.Infof("Creating Agent...")
	agentInst, err := agent.New(baseCtx, agentCfg, toolMgr, memorySvc)
	if err != nil {
		logging.Fatalf("Failed to create Agent: %v", err)
	}
	logging.Infof("Agent created successfully")

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

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	// Start AudioOutPipe (initializes internal TTSPipeline)
	logging.Infof("Starting AudioOutPipe...")
	if err := audioOutPipe.Start(ctx); err != nil {
		logging.Fatalf("Failed to start AudioOutPipe: %v", err)
	}

	// Build pipeline: ASR → Agent → TTS
	logging.Infof("Building pipeline: ASR → Agent → TTS...")
	sess := session.New(session.SessionMeta{Model: agentCfg.Model})
	pl := pipeline.NewBuilder().
		AddStage(pstages.NewASRStage(audioInPipe)).
		AddStage(pstages.NewAgentStage(agentInst, sess)).
		AddStage(pstages.NewTTSStage(audioOutPipe)).
		SetObserver(pipeline.NewLoggingObserver(true)).
		Build()

	logging.Infof("Starting pipeline...")
	if err := pl.Start(ctx); err != nil {
		logging.Fatalf("Failed to start pipeline: %v", err)
	}

	// Drain pipeline output for logging
	go func() {
		for msg := range pl.Output() {
			if msg.IsError() {
				logging.Warnf("Pipeline error [type=%s]: %v", msg.Type, msg.Metadata.Error)
			}
		}
		logging.Debugf("Pipeline output closed")
	}()

	logging.Infof("Setting up signal handler...")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logging.Infof("\n========================================")
		logging.Infof("     Received interrupt signal...       ")
		logging.Infof("========================================")

		logging.Infof("Stopping pipeline...")
		if err := pl.Stop(); err != nil {
			logging.Errorf("Error stopping pipeline: %v", err)
		}

		logging.Infof("Stopping AudioOutPipe...")
		if err := audioOutPipe.Stop(); err != nil {
			logging.Errorf("Error stopping AudioOutPipe: %v", err)
		}

		logging.Infof("Stopping Mixer...")
		if err := mixer.Stop(); err != nil {
			logging.Errorf("Error stopping mixer: %v", err)
		}

		cancel()
	}()

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
