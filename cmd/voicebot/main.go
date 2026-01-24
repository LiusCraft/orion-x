package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/internal/voicebot"
)

func main() {
	log.Println("========================================")
	log.Println("        VoiceBot Starting...           ")
	log.Println("========================================")

	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		log.Fatal("DASHSCOPE_API_KEY environment variable is required")
	}
	log.Println("API key loaded successfully")

	log.Println("Creating VoiceAgent...")
	voiceAgent, err := agent.NewVoiceAgent(context.Background())
	if err != nil {
		log.Fatalf("Failed to create VoiceAgent: %v", err)
	}
	log.Println("VoiceAgent created successfully")

	log.Println("Creating AudioMixer...")
	mixer, err := audio.NewMixer(audio.DefaultMixerConfig())
	if err != nil {
		log.Fatalf("Failed to create Mixer: %v", err)
	}
	log.Println("AudioMixer created successfully")

	log.Println("Starting AudioMixer...")
	mixer.Start()
	log.Println("AudioMixer started")

	log.Println("Creating AudioOutPipe...")
	audioOutPipe := audio.NewOutPipe(apiKey)
	audioOutPipe.SetMixer(mixer)
	log.Println("AudioOutPipe created successfully")

	log.Println("Creating AudioInPipe...")
	config := audio.DefaultInPipeConfig()

	log.Println("Creating Microphone source...")
	micSource, err := audio.NewMicrophoneSource(config.SampleRate, config.Channels, 3200)
	if err != nil {
		log.Fatalf("Failed to create Microphone source: %v", err)
	}
	log.Println("Microphone source created successfully")

	audioInPipe, err := audio.NewInPipeWithAudioSource(apiKey, config, micSource)
	if err != nil {
		log.Fatalf("Failed to create AudioInPipe: %v", err)
	}
	log.Println("AudioInPipe created successfully")

	log.Println("Creating ToolExecutor and registering tools...")
	toolExecutor := tools.NewToolExecutor()
	toolExecutor.RegisterTool("getTime", tools.GetTimeTool)
	toolExecutor.RegisterTool("getWeather", tools.GetWeatherTool)
	log.Println("Tools registered successfully")

	log.Println("Creating Orchestrator...")
	orchestrator := voicebot.NewOrchestrator(voiceAgent, audioOutPipe, audioInPipe, toolExecutor)
	log.Println("Orchestrator created successfully")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Println("Setting up signal handler...")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("\n========================================")
		log.Println("     Received interrupt signal...       ")
		log.Println("========================================")

		log.Println("Stopping Mixer...")
		mixer.Stop()

		log.Println("Stopping Orchestrator...")
		if err := orchestrator.Stop(); err != nil {
			log.Printf("Error stopping orchestrator: %v", err)
		}

		cancel()
		log.Println("Exiting...")
		os.Exit(0)
	}()

	log.Println("Starting Orchestrator...")
	if err := orchestrator.Start(ctx); err != nil {
		log.Fatalf("Failed to start orchestrator: %v", err)
	}

	log.Println("========================================")
	log.Println("     VoiceBot is Running! 🎤          ")
	log.Println("     Press Ctrl+C to stop.             ")
	log.Println("========================================")

	// Wait for signal
	<-ctx.Done()

	log.Println("\n========================================")
	log.Println("     VoiceBot Shutting Down...          ")
	log.Println("========================================")

	log.Println("VoiceBot stopped.")
}
