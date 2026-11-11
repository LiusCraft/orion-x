package realtime

import (
	"fmt"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// PipelineConfig describes the shared ASR → Agent → TTS graph used by voice
// transports. The transport owns lifecycle and output handling.
type PipelineConfig struct {
	// ASR is the processor used by the default audio input stage.
	ASR audio.ASRProcessor
	// ASRStage optionally replaces the default audio input stage.
	ASRStage pipeline.Stage
	// AudioSource feeds audio to the default ASR stage.
	AudioSource audio.AudioSource
	// Agent generates responses from ASR or text input.
	Agent agent.AgentRunner
	// Session stores conversation messages for the agent.
	Session *session.Session
	// TTS synthesizes agent output.
	TTS audio.TTSProcessor
	// Observer receives pipeline lifecycle events.
	Observer pipeline.PipelineObserver
}

// NewPipeline builds the canonical voice pipeline shared by Realtime and
// Xiaozhi transports.
func NewPipeline(cfg PipelineConfig) (pipeline.Pipeline, error) {
	if cfg.Agent == nil {
		return nil, fmt.Errorf("agent is required")
	}
	if cfg.Session == nil {
		return nil, fmt.Errorf("session is required")
	}
	if cfg.TTS == nil {
		return nil, fmt.Errorf("tts processor is required")
	}

	asrStage := cfg.ASRStage
	if asrStage == nil {
		if cfg.ASR == nil {
			return nil, fmt.Errorf("asr processor is required when no asr stage is supplied")
		}
		asrStage = audio.NewASRStage(cfg.ASR, cfg.AudioSource)
	}
	if asrStage.Name() != "asr" {
		return nil, fmt.Errorf("asr stage name = %q, want %q", asrStage.Name(), "asr")
	}
	builder := pipeline.NewDAGBuilder().
		AddStage(asrStage).
		AddStage(agent.NewAgentStage(cfg.Agent, cfg.Session)).
		AddStage(audio.NewTTSStage(cfg.TTS)).
		AddStage(audio.NewOutputStage()).
		Connect("asr", "agent").
		Connect("asr", "session_output").
		Connect("agent", "tts").
		Connect("agent", "session_output").
		Connect("tts", "session_output")
	if cfg.Observer != nil {
		builder.SetObserver(cfg.Observer)
	}
	return builder.Build()
}
