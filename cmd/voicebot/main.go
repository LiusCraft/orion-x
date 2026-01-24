package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/internal/voicebot"
)

func main() {
	if err := logging.InitFromEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logging.Sync()

	logging.SetTraceID(logging.NewTraceID())

	logging.Infof("========================================")
	logging.Infof("        VoiceBot Starting...           ")
	logging.Infof("========================================")

	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		logging.Fatalf("DASHSCOPE_API_KEY environment variable is required")
	}
	logging.Infof("API key loaded successfully")

	logging.Infof("Creating VoiceAgent...")
	voiceAgent, err := agent.NewVoiceAgent(context.Background())
	if err != nil {
		logging.Fatalf("Failed to create VoiceAgent: %v", err)
	}
	logging.Infof("VoiceAgent created successfully")

	logging.Infof("Creating AudioMixer...")
	mixer, err := audio.NewMixer(audio.DefaultMixerConfig())
	if err != nil {
		logging.Fatalf("Failed to create Mixer: %v", err)
	}
	logging.Infof("AudioMixer created successfully")

	logging.Infof("Starting AudioMixer...")
	mixer.Start()
	logging.Infof("AudioMixer started")

	logging.Infof("Creating AudioOutPipe...")
	audioOutPipe := audio.NewOutPipe(apiKey)
	audioOutPipe.SetMixer(mixer)
	logging.Infof("AudioOutPipe created successfully")

	logging.Infof("Creating AudioInPipe...")
	config := audio.DefaultInPipeConfig()

	logging.Infof("Creating Microphone source...")
	micSource, err := audio.NewMicrophoneSource(config.SampleRate, config.Channels, 3200)
	if err != nil {
		logging.Fatalf("Failed to create Microphone source: %v", err)
	}
	logging.Infof("Microphone source created successfully")

	audioInPipe, err := audio.NewInPipeWithAudioSource(apiKey, config, micSource)
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
