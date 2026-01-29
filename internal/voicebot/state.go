package voicebot

import "slices"

// StateMachine 状态机
type StateMachine struct {
	currentState State
}

func NewStateMachine() *StateMachine {
	return &StateMachine{
		currentState: StateIdle,
	}
}

// CanTransition 检查是否可以转换
func (sm *StateMachine) CanTransition(to State) bool {
	from := sm.currentState

	validTransitions := map[State][]State{
		StateIdle:       {StateListening, StateProcessing},
		StateListening:  {StateProcessing, StateIdle},
		StateProcessing: {StateSpeaking, StateIdle},
		StateSpeaking:   {StateListening, StateIdle, StateProcessing},
	}

	validTo, ok := validTransitions[from]
	if !ok {
		return false
	}

	return slices.Contains(validTo, to)
}

// Transition 状态转换
func (sm *StateMachine) Transition(to State) bool {
	if sm.CanTransition(to) {
		sm.currentState = to
		return true
	}
	return false
}

// GetCurrentState 获取当前状态
func (sm *StateMachine) GetCurrentState() State {
	return sm.currentState
}
