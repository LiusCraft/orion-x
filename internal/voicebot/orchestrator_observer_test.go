package voicebot

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/tools"
)

type mockVoiceAgent struct {
	events []agent.AgentEvent
}

func (m *mockVoiceAgent) Process(ctx context.Context, text string) (<-chan agent.AgentEvent, error) {
	ch := make(chan agent.AgentEvent, len(m.events))
	go func() {
		for _, e := range m.events {
			ch <- e
		}
		close(ch)
	}()
	return ch, nil
}

func (m *mockVoiceAgent) SummarizeToolResult(ctx context.Context, tool string, args map[string]interface{}, result interface{}) (<-chan agent.AgentEvent, error) {
	ch := make(chan agent.AgentEvent, len(m.events))
	for _, event := range m.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

func (m *mockVoiceAgent) GetToolType(tool string) agent.ToolType {
	return agent.ToolTypeQuery
}

type mockOutPipe struct {
	onFinished audio.PlaybackFinishedCallback
}

func (m *mockOutPipe) Start(ctx context.Context) error { return nil }
func (m *mockOutPipe) Stop() error                     { return nil }
func (m *mockOutPipe) SetMixer(mixer audio.AudioMixer) {}
func (m *mockOutPipe) SetOnPlaybackFinished(callback audio.PlaybackFinishedCallback) {
	m.onFinished = callback
}
func (m *mockOutPipe) SetOnTTSItemStarted(callback audio.TTSItemStartedCallback) {}
func (m *mockOutPipe) PlayTTS(text string, emotion string) error {
	if m.onFinished != nil {
		go func() {
			time.Sleep(10 * time.Millisecond)
			m.onFinished()
		}()
	}
	return nil
}
func (m *mockOutPipe) PlayResource(audio io.Reader) error { return nil }
func (m *mockOutPipe) Interrupt() error                   { return nil }
func (m *mockOutPipe) Stats() audio.PipelineStats         { return audio.PipelineStats{} }

type testObserver struct {
	events chan string
}

func (o *testObserver) OnLLMTextChunk(text, emotion string) {
	o.events <- "llm:" + text
}

func (o *testObserver) OnTTSSentence(text, emotion string) {
	o.events <- "tts_sentence:" + text
}

func (o *testObserver) OnTTSStart() {
	o.events <- "tts_start"
}

func (o *testObserver) OnTTSStop(isAborted bool) {
	if isAborted {
		o.events <- "tts_stop_aborted"
	} else {
		o.events <- "tts_stop"
	}
}

func TestOrchestratorObserver(t *testing.T) {
	voice := &mockVoiceAgent{
		events: []agent.AgentEvent{
			&agent.TextChunkEvent{Chunk: "hi.", Emotion: "happy"},
			&agent.FinishedEvent{Error: nil},
		},
	}
	outPipe := &mockOutPipe{}
	toolExec := tools.NewToolExecutor()
	observer := &testObserver{events: make(chan string, 10)}

	orchestrator := NewOrchestratorWithOptions(
		voice,
		outPipe,
		nil,
		toolExec,
		&OrchestratorOptions{Observer: observer},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := orchestrator.Start(ctx); err != nil {
		t.Fatalf("start orchestrator failed: %v", err)
	}
	defer orchestrator.Stop()

	orchestrator.OnASRFinal("hi")

	want := []string{"llm:hi.", "tts_start", "tts_sentence:hi.", "tts_stop"}
	got := make([]string, 0, len(want))

	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	for len(got) < len(want) {
		select {
		case ev := <-observer.events:
			got = append(got, ev)
		case <-timer.C:
			t.Fatalf("timeout waiting for events, got %v", got)
		}
	}

	for i, expected := range want {
		if got[i] != expected {
			t.Fatalf("event %d mismatch: got %s want %s", i, got[i], expected)
		}
	}
}
