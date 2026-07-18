package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// SubAgent runs one independent agent turn in the background. Its output is
// exposed as pipeline messages so task mounts can forward it into sessions.
type SubAgent struct {
	ID     string
	TaskID string

	agent AgentRunner

	mu       sync.Mutex
	cancel   context.CancelFunc
	OutputCh chan pipeline.Message
}

func NewSubAgent(id, taskID string, agt AgentRunner) *SubAgent {
	return &SubAgent{ID: id, TaskID: taskID, agent: agt, OutputCh: make(chan pipeline.Message, 32)}
}

// Start begins a background run using the supplied session. Calling Start
// more than once while it is active is ignored.
func (sa *SubAgent) Start(ctx context.Context, sess *session.Session) error {
	if sa == nil || sa.agent == nil {
		return errors.New("sub-agent requires an agent")
	}
	if sess == nil {
		return errors.New("sub-agent requires a session")
	}
	sa.mu.Lock()
	if sa.cancel != nil {
		sa.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	sa.cancel = cancel
	sa.mu.Unlock()

	events, err := sa.agent.Run(runCtx, sess)
	if err != nil {
		cancel()
		sa.mu.Lock()
		sa.cancel = nil
		sa.mu.Unlock()
		return err
	}
	go sa.forward(runCtx, events)
	return nil
}

func (sa *SubAgent) forward(ctx context.Context, events <-chan AgentEvent) {
	defer close(sa.OutputCh)
	defer func() {
		sa.mu.Lock()
		sa.cancel = nil
		sa.mu.Unlock()
	}()
	for event := range events {
		var msg pipeline.Message
		switch e := event.(type) {
		case *TextChunkEvent:
			msg = pipeline.NewMessage(pipeline.MessageTypeData, e.Chunk)
			msg.Metadata.Extra = map[string]interface{}{"source": "sub_agent", "task_id": sa.TaskID}
		case *FinishedEvent:
			if e.Error != nil {
				msg = pipeline.NewMessage(pipeline.MessageTypeError, nil).WithError(e.Error)
			} else {
				msg = pipeline.NewMessage(pipeline.MessageTypeFinished, nil)
			}
		default:
			continue
		}
		select {
		case sa.OutputCh <- msg:
		case <-ctx.Done():
			return
		}
	}
}

func (sa *SubAgent) Stop() {
	sa.mu.Lock()
	cancel := sa.cancel
	sa.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
