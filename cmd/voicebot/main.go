package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gordonklaus/portaudio"
	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/config"
	_ "github.com/liuscraft/orion-x/internal/llm/provider/anthropic/messages"
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai"
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai/responses"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/provider/asr"
	_ "github.com/liuscraft/orion-x/internal/provider/asr/register"
	"github.com/liuscraft/orion-x/internal/provider/tts"
	_ "github.com/liuscraft/orion-x/internal/provider/tts/register"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/pkg/pipeline"
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
	ttsCfgSpec := appConfig.Provider.TTS.Aliyun
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
		MemoryCharLimit: appConfig.Memory.MemoryCharLimit,
		UserCharLimit:   appConfig.Memory.UserCharLimit,
	}
	memorySvc, err := memory.NewService(memCfg, memory.Options{
		ManagerURL:   "",
		DeviceID:     "local",
		ReviewConfig: memory.ReviewConfig{Enabled: false},
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
		Provider:        appConfig.Provider.LLM.Type,
		APIKey:          llmCfg.APIKey,
		BaseURL:         llmCfg.BaseURL,
		Model:           llmCfg.Model,
		SoulPrompt:      llmCfg.SoulPrompt,
		RulesPrompt:     llmCfg.RulesPrompt,
		ExtraFields:     llmCfg.ExtraFields,
		ProviderOptions: llmCfg.Options,
		Thinking:        llmCfg.Thinking,
		MaxOutputTokens: llmCfg.MaxOutputTokens,
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

	logging.Infof("Initializing PortAudio...")
	if err := portaudio.Initialize(); err != nil {
		logging.Fatalf("Failed to initialize PortAudio: %v", err)
	}
	// portaudio.Terminate() crashes on macOS due to C++ static destructor
	// ordering issues (mutex lock failed: Invalid argument).
	logging.Infof("PortAudio initialized successfully")

	// --- ASR ---
	logging.Infof("Creating ASR Recognizer...")
	recognizer, err := asr.NewRecognizer(asr.ProviderConfig{
		Type: appConfig.Provider.ASR.Type,
		Config: asr.Config{
			APIKey:     asrCfg.APIKey,
			Endpoint:   asrCfg.Endpoint,
			Model:      asrCfg.Model,
			Format:     "pcm",
			SampleRate: audio.InternalSampleRate,
		},
	})
	if err != nil {
		logging.Fatalf("Failed to create ASR recognizer: %v", err)
	}

	logging.Infof("Creating ASRProcessor...")
	inPipeCfg := appConfig.Audio.InPipe
	asrProc, err := audio.NewASRProcessor(&audio.ASRConfig{
		EnableVAD:       inPipeCfg.EnableVAD,
		VADThreshold:    inPipeCfg.VADThreshold,
		VADType:         inPipeCfg.VADType,
		VADModelPath:    inPipeCfg.VADModelPath,
		VADMinSilenceMs: inPipeCfg.VADMinSilenceMs,
		VADSpeechPadMs:  inPipeCfg.VADSpeechPadMs,
		Recognizer:      recognizer,
	})
	if err != nil {
		logging.Fatalf("Failed to create ASRProcessor: %v", err)
	}
	logging.Infof("ASRProcessor created successfully")

	// --- TTS ---
	logging.Infof("Creating TTS Provider...")
	ttsProvider, err := tts.NewProvider(tts.ProviderConfig{
		Type: appConfig.Provider.TTS.Type,
		Config: tts.Config{
			APIKey:               ttsCfgSpec.APIKey,
			Endpoint:             ttsCfgSpec.Endpoint,
			Workspace:            ttsCfgSpec.Workspace,
			Model:                ttsCfgSpec.Model,
			Voice:                ttsCfgSpec.Voice,
			Format:               "pcm",
			SampleRate:           ttsCfgSpec.SampleRate,
			Volume:               ttsCfgSpec.Volume,
			Rate:                 ttsCfgSpec.Rate,
			Pitch:                ttsCfgSpec.Pitch,
			EnableSSML:           ttsCfgSpec.EnableSSML,
			TextType:             ttsCfgSpec.TextType,
			EnableDataInspection: ttsCfgSpec.EnableDataInspection,
		},
	})
	if err != nil {
		logging.Fatalf("Failed to create TTS provider: %v", err)
	}

	logging.Infof("Creating TTSProcessor...")
	ttsPipeCfg := appConfig.Audio.TTSPipeline
	maxConcurrent := ttsPipeCfg.MaxConcurrentTTS
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	queueSize := ttsPipeCfg.TextQueueSize
	if queueSize <= 0 {
		queueSize = 100
	}

	processorCfg := audio.DefaultTTSConfig()
	processorCfg.Provider = ttsProvider
	processorCfg.QueueSize = queueSize

	ttsProc, err := audio.NewTTSProcessor(processorCfg)
	if err != nil {
		logging.Fatalf("Failed to create TTSProcessor: %v", err)
	}
	logging.Infof("TTSProcessor created (maxConcurrent=%d)", maxConcurrent)

	// --- Microphone source ---
	bufferSize := inPipeCfg.BufferSize
	if bufferSize <= 0 {
		bufferSize = 3200
	}

	sampleRate := inPipeCfg.SampleRate
	if sampleRate <= 0 {
		sampleRate = audio.InternalSampleRate
	}
	channels := inPipeCfg.Channels
	if channels <= 0 {
		channels = audio.InternalChannels
	}

	logging.Infof("Creating Microphone source (bufferSize=%d, highLatency=%v, inputDevice=%q)...",
		bufferSize, inPipeCfg.HighLatency, inPipeCfg.InputDevice)

	inputDevice := inPipeCfg.InputDevice
	if inputDevice == "" {
		selected, err := SelectInputDevice()
		if err != nil {
			logging.Warnf("Failed to select input device: %v, using default", err)
		} else {
			inputDevice = selected
		}
	}

	micSource, err := NewMicrophoneSourceWithDevice(
		sampleRate,
		channels,
		bufferSize,
		inPipeCfg.HighLatency,
		inputDevice,
	)
	if err != nil {
		logging.Fatalf("Failed to create Microphone source: %v", err)
	}
	logging.Infof("Microphone source created successfully")

	// --- PortAudio sink ---
	sink := NewPortAudioSink()

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	sinkFormat := AudioFormat{
		SampleRate:      ttsCfgSpec.SampleRate,
		Channels:        audio.InternalChannels,
		FramesPerBuffer: 1024,
	}
	logging.Infof("Starting PortAudio sink...")
	if err := sink.Start(ctx, sinkFormat); err != nil {
		logging.Fatalf("Failed to start sink: %v", err)
	}

	// Start TTSProcessor (ASRProcessor is started inside ASRStage).
	logging.Infof("Starting TTSProcessor...")
	if err := ttsProc.Start(ctx); err != nil {
		logging.Fatalf("Failed to start TTSProcessor: %v", err)
	}

	// Build pipeline: ASR → Agent → TTS → PortAudio output.
	// TTS audio flows through the pipeline Message bus (TTSStage registers
	// proc.OnChunk internally); PortAudioOutputStage is the sink that writes
	// it to the speaker.
	logging.Infof("Building pipeline: ASR → Agent → TTS → PortAudio...")
	sess := session.New(session.SessionMeta{Model: agentCfg.Model})
	pl := pipeline.NewBuilder().
		AddStage(audio.NewASRStage(asrProc, micSource)).
		AddStage(agent.NewAgentStage(agentInst, sess)).
		AddStage(audio.NewTTSStage(ttsProc)).
		AddStage(NewPortAudioOutputStage(sink, sinkFormat.SampleRate)).
		SetObserver(pipeline.NewLoggingObserver(false)).
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

		logging.Infof("Stopping TTSProcessor...")
		if err := ttsProc.Stop(); err != nil {
			logging.Errorf("Error stopping TTSProcessor: %v", err)
		}

		logging.Infof("Stopping sink...")
		if err := sink.Stop(); err != nil {
			logging.Errorf("Error stopping sink: %v", err)
		}

		cancel()
	}()

	logging.Infof("========================================")
	logging.Infof("     VoiceBot is Running! 🎤          ")
	logging.Infof("     Press Ctrl+C to stop.             ")
	logging.Infof("========================================")

	<-ctx.Done()

	logging.Infof("\n========================================")
	logging.Infof("     VoiceBot Shutting Down...          ")
	logging.Infof("========================================")

	logging.Infof("VoiceBot stopped.")

	logging.Sync()
	// Raw _exit syscall bypasses C/C++ library cleanup to avoid
	// PortAudio CoreAudio thread destructor crash on macOS.
	_, _, _ = syscall.Syscall(syscall.SYS_EXIT, 0, 0, 0)
}

func toToolsMCPServers(cfgs []config.MCPServerConfig) []tools.MCPServerConfig {
	servers := make([]tools.MCPServerConfig, 0, len(cfgs))
	for _, cfg := range cfgs {
		servers = append(servers, tools.MCPServerConfig{
			ID:           cfg.ID,
			Transport:    cfg.Transport,
			Command:      cfg.Command,
			Args:         cfg.Args,
			Env:          cfg.Env,
			CWD:          cfg.CWD,
			Endpoint:     cfg.Endpoint,
			Headers:      cfg.Headers,
			ToolNameList: cfg.ToolNameList,
			TimeoutMs:    cfg.TimeoutMs,
		})
	}
	return servers
}
