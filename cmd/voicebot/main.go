package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/audio/source"
	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/internal/tts"
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

	toolTypes, err := agent.ParseToolTypes(appConfig.Tools.Types)
	if err != nil {
		logging.Fatalf("Invalid tool types: %v", err)
	}

	logging.Infof("Creating VoiceAgent...")
	voiceAgent, err := agent.NewVoiceAgentWithConfig(context.Background(), agent.Config{
		APIKey:          appConfig.LLM.APIKey,
		BaseURL:         appConfig.LLM.BaseURL,
		Model:           appConfig.LLM.Model,
		ToolTypes:       toolTypes,
		ActionResponses: appConfig.Tools.ActionResponses,
	})
	if err != nil {
		logging.Fatalf("Failed to create VoiceAgent: %v", err)
	}
	logging.Infof("VoiceAgent created successfully")

	logging.Infof("Creating AudioMixer...")
	mixerCfg := &audio.MixerConfig{
		TTSVolume:      appConfig.Audio.Mixer.TTSVolume,
		ResourceVolume: appConfig.Audio.Mixer.ResourceVolume,
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
	mixer.Start()
	logging.Infof("AudioMixer started")

	logging.Infof("Creating AudioOutPipe...")
	outPipeCfg := audio.DefaultOutPipeConfig()
	outPipeCfg.Mixer = mixerCfg
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
		APIKey:               appConfig.TTS.APIKey,
		Endpoint:             appConfig.TTS.Endpoint,
		Workspace:            appConfig.TTS.Workspace,
		Model:                appConfig.TTS.Model,
		Voice:                appConfig.TTS.Voice,
		Format:               appConfig.TTS.Format,
		SampleRate:           appConfig.TTS.SampleRate,
		Volume:               appConfig.TTS.Volume,
		Rate:                 appConfig.TTS.Rate,
		Pitch:                appConfig.TTS.Pitch,
		EnableSSML:           appConfig.TTS.EnableSSML,
		TextType:             appConfig.TTS.TextType,
		EnableDataInspection: appConfig.TTS.EnableDataInspection,
	}
	if len(appConfig.TTS.VoiceMap) > 0 {
		outPipeCfg.VoiceMap = appConfig.TTS.VoiceMap
	}
	audioOutPipe := audio.NewOutPipeWithConfig(outPipeCfg)
	audioOutPipe.SetMixer(mixer)
	logging.Infof("AudioOutPipe created successfully (async TTS pipeline: maxBuffer=%d, maxConcurrent=%d)",
		outPipeCfg.TTSPipeline.MaxTTSBuffer, outPipeCfg.TTSPipeline.MaxConcurrentTTS)

	logging.Infof("Creating AudioInPipe...")
	inPipeCfg := &audio.InPipeConfig{
		SampleRate:   appConfig.Audio.InPipe.SampleRate,
		Channels:     appConfig.Audio.InPipe.Channels,
		EnableVAD:    appConfig.Audio.InPipe.EnableVAD,
		VADThreshold: appConfig.Audio.InPipe.VADThreshold,
		ASRModel:     appConfig.ASR.Model,
		ASREndpoint:  appConfig.ASR.Endpoint,
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

	aecCfg := audio.DefaultEchoCancelConfig()
	aecCfg.Enabled = appConfig.Audio.InPipe.AEC.Enable
	aecCfg.Mode = appConfig.Audio.InPipe.AEC.Mode
	if appConfig.Audio.InPipe.AEC.FrameMs > 0 {
		aecCfg.FrameMs = appConfig.Audio.InPipe.AEC.FrameMs
	}
	if appConfig.Audio.InPipe.AEC.FarEndDelayMs > 0 {
		aecCfg.FarEndDelayMs = appConfig.Audio.InPipe.AEC.FarEndDelayMs
	}
	if appConfig.Audio.InPipe.AEC.ReferenceActiveWindowMs > 0 {
		aecCfg.ReferenceActiveWindowMs = appConfig.Audio.InPipe.AEC.ReferenceActiveWindowMs
	}

	audioSource := audio.AudioSource(micSource)
	if aecCfg.Enabled {
		frameBytes := audio.FrameBytes(inPipeCfg.SampleRate, inPipeCfg.Channels, aecCfg.FrameMs)
		delayFrames := 0
		if aecCfg.FrameMs > 0 {
			delayFrames = aecCfg.FarEndDelayMs / aecCfg.FrameMs
		}
		referenceBuffer := audio.NewReferenceBuffer(frameBytes, 200, delayFrames)
		referenceBuffer.SetActiveWindow(time.Duration(aecCfg.ReferenceActiveWindowMs) * time.Millisecond)
		audioOutPipe.SetReferenceSink(referenceBuffer)
		audioSource = audio.NewEchoCancellingSource(
			micSource,
			aecCfg,
			referenceBuffer,
			audio.NewNoopEchoCanceller(),
			inPipeCfg.SampleRate,
			inPipeCfg.Channels,
		)
	}

	audioInPipe, err := audio.NewInPipeWithAudioSource(appConfig.ASR.APIKey, inPipeCfg, audioSource)
	if err != nil {
		logging.Fatalf("Failed to create AudioInPipe: %v", err)
	}
	logging.Infof("AudioInPipe created successfully")

	logging.Infof("Creating ToolExecutor and registering tools...")
	toolExecutor := tools.NewToolExecutor()
	toolExecutor.RegisterTool("getTime", tools.GetTimeTool)
	toolExecutor.RegisterTool("getWeather", tools.GetWeatherTool)
	logging.Infof("Tools registered successfully")

	logging.Infof("Creating Orchestrator...")
	orchestrator := voicebot.NewOrchestrator(voiceAgent, audioOutPipe, audioInPipe, toolExecutor)
	logging.Infof("Orchestrator created successfully")

	ctx, cancel := context.WithCancel(context.Background())
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
		mixer.Stop()

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
