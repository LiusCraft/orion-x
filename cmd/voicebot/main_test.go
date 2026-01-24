package main

import (
	"context"
	"testing"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/internal/voicebot"
)

func TestVoiceBotIntegration(t *testing.T) {
	ctx := context.Background()

	voiceAgent, err := agent.NewVoiceAgent(ctx)
	if err != nil {
		if err.Error() == "DASHSCOPE_API_KEY environment variable is required" {
			t.Skip("Skipping integration test: DASHSCOPE_API_KEY not set")
		}
		t.Fatalf("Failed to create VoiceAgent: %v", err)
	}

	mixer, err := audio.NewMixer(audio.DefaultMixerConfig())
	if err != nil {
		t.Fatalf("Failed to create Mixer: %v", err)
	}

	audioOutPipe := audio.NewOutPipe("test-key")
	audioOutPipe.SetMixer(mixer)

	audioInPipe, err := audio.NewInPipe("test-key", audio.DefaultInPipeConfig())
	if err != nil {
		t.Fatalf("Failed to create AudioInPipe: %v", err)
	}

	toolExecutor := tools.NewToolExecutor()
	toolExecutor.RegisterTool("getTime", tools.GetTimeTool)
	toolExecutor.RegisterTool("getWeather", tools.GetWeatherTool)

	orchestrator := voicebot.NewOrchestrator(voiceAgent, audioOutPipe, audioInPipe, toolExecutor)

	if err := orchestrator.Start(ctx); err != nil {
		t.Fatalf("Failed to start orchestrator: %v", err)
	}
	defer orchestrator.Stop()

	t.Log("Orchestrator started successfully")

	state := orchestrator.GetState()
	if state != voicebot.StateIdle {
		t.Errorf("Expected state Idle, got %s", state)
	}
}
