package voicebot

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/tools"
)

type schedulerOutPipe struct {
	mu         sync.Mutex
	onFinished audio.PlaybackFinishedCallback
	playCalls  []string
}

func (m *schedulerOutPipe) Start(ctx context.Context) error { return nil }
func (m *schedulerOutPipe) Stop() error                     { return nil }
func (m *schedulerOutPipe) SetMixer(mixer audio.AudioMixer) {}
func (m *schedulerOutPipe) SetOnPlaybackFinished(callback audio.PlaybackFinishedCallback) {
	m.mu.Lock()
	m.onFinished = callback
	m.mu.Unlock()
}
func (m *schedulerOutPipe) SetOnTTSItemStarted(callback audio.TTSItemStartedCallback) {}
func (m *schedulerOutPipe) PlayTTS(text string, emotion string) error {
	m.mu.Lock()
	m.playCalls = append(m.playCalls, text)
	m.mu.Unlock()
	return nil
}
func (m *schedulerOutPipe) PlayResource(audio io.Reader) error { return nil }
func (m *schedulerOutPipe) Interrupt() error                   { return nil }
func (m *schedulerOutPipe) Stats() audio.PipelineStats         { return audio.PipelineStats{} }

func (m *schedulerOutPipe) triggerFinished() {
	m.mu.Lock()
	callback := m.onFinished
	m.mu.Unlock()
	if callback != nil {
		callback()
	}
}

func (m *schedulerOutPipe) playCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.playCalls)
}

func (m *schedulerOutPipe) playOrder() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.playCalls))
	copy(out, m.playCalls)
	return out
}

func TestTTSSchedulerMaxInFlight(t *testing.T) {
	outPipe := &schedulerOutPipe{}
	orchestrator := NewOrchestratorWithOptions(
		nil,
		outPipe,
		nil,
		tools.NewToolExecutor(),
		&OrchestratorOptions{
			TTSScheduler: TTSSchedulerConfig{MaxInFlightSentences: 1, MaxCacheSentences: 0},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := orchestrator.Start(ctx); err != nil {
		t.Fatalf("start orchestrator failed: %v", err)
	}
	defer orchestrator.Stop()

	impl := orchestrator.(*orchestratorImpl)
	impl.cacheSentence("one", "default")
	impl.cacheSentence("two", "default")
	impl.cacheSentence("three", "default")

	if got := outPipe.playCount(); got != 1 {
		t.Fatalf("expected 1 play call, got %d", got)
	}

	outPipe.triggerFinished()

	if got := outPipe.playCount(); got != 2 {
		t.Fatalf("expected 2 play calls after finish, got %d", got)
	}
}

func TestTTSSchedulerInterruptClearsQueues(t *testing.T) {
	outPipe := &schedulerOutPipe{}
	orchestrator := NewOrchestratorWithOptions(
		nil,
		outPipe,
		nil,
		tools.NewToolExecutor(),
		&OrchestratorOptions{
			TTSScheduler: TTSSchedulerConfig{MaxInFlightSentences: 2, MaxCacheSentences: 0},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := orchestrator.Start(ctx); err != nil {
		t.Fatalf("start orchestrator failed: %v", err)
	}
	defer orchestrator.Stop()

	impl := orchestrator.(*orchestratorImpl)
	impl.cacheSentence("one", "default")
	impl.cacheSentence("two", "default")

	orchestrator.OnUserSpeakingDetected()

	impl.mu.Lock()
	if len(impl.pendingQueue) != 0 || len(impl.inFlightQueue) != 0 {
		impl.mu.Unlock()
		t.Fatalf("expected queues to be cleared, pending=%d inflight=%d", len(impl.pendingQueue), len(impl.inFlightQueue))
	}
	if impl.ttsPendingCount != 0 {
		impl.mu.Unlock()
		t.Fatalf("expected ttsPendingCount to be 0, got %d", impl.ttsPendingCount)
	}
	for _, rec := range impl.sentenceCache {
		if rec.Status == SentencePending || rec.Status == SentenceEnqueued {
			impl.mu.Unlock()
			t.Fatalf("expected cached sentence to be aborted, got %s", rec.Status)
		}
	}
	impl.mu.Unlock()
}

func TestTTSSchedulerOrder(t *testing.T) {
	outPipe := &schedulerOutPipe{}
	orchestrator := NewOrchestratorWithOptions(
		nil,
		outPipe,
		nil,
		tools.NewToolExecutor(),
		&OrchestratorOptions{
			TTSScheduler: TTSSchedulerConfig{MaxInFlightSentences: 2, MaxCacheSentences: 0},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := orchestrator.Start(ctx); err != nil {
		t.Fatalf("start orchestrator failed: %v", err)
	}
	defer orchestrator.Stop()

	impl := orchestrator.(*orchestratorImpl)
	impl.cacheSentence("first", "default")
	impl.cacheSentence("second", "default")
	impl.cacheSentence("third", "default")

	got := outPipe.playOrder()
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("unexpected order before finish: %v", got)
	}

	outPipe.triggerFinished()

	got = outPipe.playOrder()
	if len(got) != 3 || got[2] != "third" {
		t.Fatalf("unexpected order after finish: %v", got)
	}
}

func TestTTSSchedulerNewTurnClearsQueues(t *testing.T) {
	outPipe := &schedulerOutPipe{}
	voice := &mockVoiceAgent{events: nil}
	orchestrator := NewOrchestratorWithOptions(
		voice,
		outPipe,
		nil,
		tools.NewToolExecutor(),
		&OrchestratorOptions{
			TTSScheduler: TTSSchedulerConfig{MaxInFlightSentences: 1, MaxCacheSentences: 0},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := orchestrator.Start(ctx); err != nil {
		t.Fatalf("start orchestrator failed: %v", err)
	}
	defer orchestrator.Stop()

	impl := orchestrator.(*orchestratorImpl)
	impl.cacheSentence("old", "default")

	oldTurn := impl.currentTurnID
	impl.handleASRFinal(NewASRFinalEvent("new"))

	impl.mu.Lock()
	if impl.currentTurnID != oldTurn+1 {
		impl.mu.Unlock()
		t.Fatalf("expected turn increment, got %d want %d", impl.currentTurnID, oldTurn+1)
	}
	if len(impl.pendingQueue) != 0 || len(impl.inFlightQueue) != 0 {
		impl.mu.Unlock()
		t.Fatalf("expected queues cleared on new turn, pending=%d inflight=%d", len(impl.pendingQueue), len(impl.inFlightQueue))
	}
	foundAborted := false
	for _, rec := range impl.sentenceCache {
		if rec.Text == "old" && rec.Status == SentenceAborted {
			foundAborted = true
			break
		}
	}
	impl.mu.Unlock()
	if !foundAborted {
		t.Fatalf("expected old sentence to be aborted on new turn")
	}
}

func TestTTSSchedulerSkipsPunctuation(t *testing.T) {
	outPipe := &schedulerOutPipe{}
	orchestrator := NewOrchestratorWithOptions(
		nil,
		outPipe,
		nil,
		tools.NewToolExecutor(),
		&OrchestratorOptions{
			TTSScheduler: TTSSchedulerConfig{MaxInFlightSentences: 1, MaxCacheSentences: 0},
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := orchestrator.Start(ctx); err != nil {
		t.Fatalf("start orchestrator failed: %v", err)
	}
	defer orchestrator.Stop()

	impl := orchestrator.(*orchestratorImpl)
	impl.cacheSentence("”", "default")

	if got := outPipe.playCount(); got != 0 {
		t.Fatalf("expected no play call for punctuation-only sentence, got %d", got)
	}

	impl.mu.Lock()
	defer impl.mu.Unlock()
	if len(impl.sentenceCache) != 1 {
		t.Fatalf("expected sentence to be cached, got %d", len(impl.sentenceCache))
	}
	if impl.sentenceCache[0].Status != SentenceSkipped {
		t.Fatalf("expected sentence status skipped, got %s", impl.sentenceCache[0].Status)
	}
}
