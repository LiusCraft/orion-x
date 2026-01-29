package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	mixer, err := audio.NewMixer(mixerCfg)
	if err != nil {
		logging.Fatalf("Failed to create Mixer: %v", err)
	}
	logging.Infof("AudioMixer created successfully")

	logging.Infof("Starting AudioMixer...")
	mixer.Start()
	logging.Infof("AudioMixer started")

	logging.Infof("Creating AudioOutPipe...")
	outPipeCfg := audio.DefaultOutPipeConfig()
	outPipeCfg.Mixer = mixerCfg
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
	logging.Infof("AudioOutPipe created successfully")

	logging.Infof("Creating AudioInPipe...")
	inPipeCfg := &audio.InPipeConfig{
		SampleRate:   appConfig.Audio.InPipe.SampleRate,
		Channels:     appConfig.Audio.InPipe.Channels,
		EnableVAD:    appConfig.Audio.InPipe.EnableVAD,
		VADThreshold: appConfig.Audio.InPipe.VADThreshold,
		ASRModel:     appConfig.ASR.Model,
		ASREndpoint:  appConfig.ASR.Endpoint,
	}

	logging.Infof("Creating Microphone source...")
	micSource, err := source.NewMicrophoneSource(inPipeCfg.SampleRate, inPipeCfg.Channels, 3200)
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

		logging.Infof("Stopping Mixer...")
		mixer.Stop()

		logging.Infof("Stopping Orchestrator...")
		if err := orchestrator.Stop(); err != nil {
			logging.Errorf("Error stopping orchestrator: %v", err)
		}

		cancel()
		logging.Infof("Exiting...")
		logging.Sync()
		os.Exit(0)
	}()

	logging.Infof("Starting Orchestrator...")
	if err := orchestrator.Start(ctx); err != nil {
		logging.Fatalf("Failed to start orchestrator: %v", err)
	}

	logging.Infof("========================================")
	logging.Infof("     VoiceBot is Running! 🎤          ")
	logging.Infof("     Press Ctrl+C to stop.             ")
	logging.Infof("========================================")

	// Wait for signal
	<-ctx.Done()

	logging.Infof("\n========================================")
	logging.Infof("     VoiceBot Shutting Down...          ")
	logging.Infof("========================================")

	logging.Infof("VoiceBot stopped.")
}
