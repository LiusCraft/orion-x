package voicebot

import (
	"context"
	"sync"
	"testing"
)

func TestStateMachine(t *testing.T) {
	sm := NewStateMachine()

	tests := []struct {
		name          string
		from          State
		to            State
		shouldSucceed bool
	}{
		{"Idle to Listening", StateIdle, StateListening, true},
		{"Idle to Processing", StateIdle, StateProcessing, true},
		{"Listening to Processing", StateListening, StateProcessing, true},
		{"Listening to Idle", StateListening, StateIdle, true},
		{"Processing to Speaking", StateProcessing, StateSpeaking, true},
		{"Processing to Idle", StateProcessing, StateIdle, true},
		{"Speaking to Listening", StateSpeaking, StateListening, true},
		{"Speaking to Idle", StateSpeaking, StateIdle, true},
		{"Speaking to Processing", StateSpeaking, StateProcessing, true},
		{"Idle to Speaking", StateIdle, StateSpeaking, false},
		{"Listening to Speaking", StateListening, StateSpeaking, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm.currentState = tt.from
			result := sm.Transition(tt.to)
			if result != tt.shouldSucceed {
				t.Errorf("Transition(%v) = %v, want %v", tt.to, result, tt.shouldSucceed)
			}
			if result && sm.GetCurrentState() != tt.to {
				t.Errorf("State after transition = %v, want %v", sm.GetCurrentState(), tt.to)
			}
		})
	}
}

func TestCanTransition(t *testing.T) {
	sm := NewStateMachine()

	tests := []struct {
		name     string
		from     State
		to       State
		expected bool
	}{
		{"Idle can go to Listening", StateIdle, StateListening, true},
		{"Idle can go to Processing", StateIdle, StateProcessing, true},
		{"Idle cannot go to Speaking", StateIdle, StateSpeaking, false},
		{"Speaking can go to Listening", StateSpeaking, StateListening, true},
		{"Speaking can go to Idle", StateSpeaking, StateIdle, true},
		{"Speaking can go to Processing", StateSpeaking, StateProcessing, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm.currentState = tt.from
			result := sm.CanTransition(tt.to)
			if result != tt.expected {
				t.Errorf("CanTransition(%v) = %v, want %v", tt.to, result, tt.expected)
			}
		})
	}
}

func TestEventBus(t *testing.T) {
	eb := NewEventBus()

	tests := []struct {
		name      string
		eventType EventType
		event     Event
	}{
		{"UserSpeakingDetected", EventTypeUserSpeakingDetected, NewUserSpeakingDetectedEvent()},
		{"ASRFinal", EventTypeASRFinal, NewASRFinalEvent("test")},
		{"ToolCallRequested", EventTypeToolCallRequested, NewToolCallRequestedEvent("tool", nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wg sync.WaitGroup
			wg.Add(1)

			received := false
			handler := func(event Event) {
				defer wg.Done()
				if event.Type() == tt.eventType {
					received = true
				}
			}

			eb.Subscribe(tt.eventType, handler)
			eb.Publish(tt.event)

			wg.Wait()

			if !received {
				t.Error("Event was not received by handler")
			}
		})
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateIdle, "Idle"},
		{StateListening, "Listening"},
		{StateProcessing, "Processing"},
		{StateSpeaking, "Speaking"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			if result != tt.expected {
				t.Errorf("State.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestOrchestratorCreation(t *testing.T) {
	orch := NewOrchestrator(nil, nil, nil, nil)
	if orch == nil {
		t.Error("NewOrchestrator returned nil")
	}

	impl, ok := orch.(*orchestratorImpl)
	if !ok {
		t.Error("NewOrchestrator did not return *orchestratorImpl")
	}

	if impl == nil {
		t.Error("impl is nil")
	}
}

func TestOrchestratorInitialState(t *testing.T) {
	orch := NewOrchestrator(nil, nil, nil, nil)
	state := orch.GetState()
	if state != StateIdle {
		t.Errorf("Initial state = %v, want %v", state, StateIdle)
	}
}

func TestOrchestratorStartStop(t *testing.T) {
	orch := NewOrchestrator(nil, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := orch.Start(ctx)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	err = orch.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}
