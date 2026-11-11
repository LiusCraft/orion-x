package voice

import (
	"context"
	"testing"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/provider/tts"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

func TestNewPipelineRequiresComponents(t *testing.T) {
	if _, err := NewPipeline(PipelineConfig{}); err == nil {
		t.Fatal("NewPipeline() error = nil, want missing agent error")
	}
}

func TestNewPipelineAcceptsReplacementASRStage(t *testing.T) {
	stage := testPipelineStage{name: "asr"}
	_, err := NewPipeline(PipelineConfig{
		ASRStage: &stage,
		Agent:    &testAgentRunner{},
		Session:  session.New(session.SessionMeta{}),
		TTS:      &testTTSProcessor{},
	})
	if err != nil {
		t.Fatalf("NewPipeline() error = %v, want nil", err)
	}
}

func TestNewPipelineRejectsReplacementASRStageWithWrongName(t *testing.T) {
	stage := testPipelineStage{name: "custom_asr"}
	_, err := NewPipeline(PipelineConfig{
		ASRStage: &stage,
		Agent:    &testAgentRunner{},
		Session:  session.New(session.SessionMeta{}),
		TTS:      &testTTSProcessor{},
	})
	if err == nil {
		t.Fatal("NewPipeline() error = nil, want invalid ASR stage name")
	}
}

type testPipelineStage struct {
	name string
}

func (s *testPipelineStage) Name() string {
	return s.name
}

func (s *testPipelineStage) Process(context.Context, <-chan pipeline.Message) <-chan pipeline.Message {
	return nil
}

type testAgentRunner struct{}

func (*testAgentRunner) Run(context.Context, *session.Session) (<-chan agent.AgentEvent, error) {
	ch := make(chan agent.AgentEvent)
	close(ch)
	return ch, nil
}

func (*testAgentRunner) SetLanguage(string) {}

type testTTSProcessor struct{}

func (*testTTSProcessor) Write(string, tts.SynthesisOptions) error { return nil }
func (*testTTSProcessor) Flush(tts.SynthesisOptions) error         { return nil }
func (*testTTSProcessor) OnChunk(func(audio.TTSChunk))             {}
func (*testTTSProcessor) OnSynthesis(func(audio.Synthesis))        {}
func (*testTTSProcessor) Interrupt() error                         { return nil }
func (*testTTSProcessor) Start(context.Context) error              { return nil }
func (*testTTSProcessor) Stop() error                              { return nil }
