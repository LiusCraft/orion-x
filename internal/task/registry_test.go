package task

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

type capturePipeline struct {
	mu        sync.Mutex
	emitted   []pipeline.Message
	emitReady chan struct{}
}

func TestTaskToolSpecsCreateAndMount(t *testing.T) {
	r := NewRegistry()
	started := false
	registry := tools.NewRegistry(r.ToolSpecs("session-1", func(_ context.Context, created *Task) error {
		started = created.Title == "Research itinerary"
		return nil
	})...)
	result, err := registry.Execute(context.Background(), "CreateTask", json.RawMessage(`{"title":"Research itinerary"}`))
	if err != nil {
		t.Fatalf("CreateTask error = %v", err)
	}
	var created Task
	if err := json.Unmarshal([]byte(result.Output), &created); err != nil {
		t.Fatalf("decode result = %v", err)
	}
	if !started || len(created.MountedSessions) != 1 || created.MountedSessions[0] != "session-1" {
		t.Fatalf("created task = %#v, started = %t", created, started)
	}
	result, err = registry.Execute(context.Background(), "GetTaskProgress", json.RawMessage(`{"task_id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("GetTaskProgress error = %v", err)
	}
	if result.Output == "" {
		t.Fatal("GetTaskProgress returned an empty result")
	}
}

func (p *capturePipeline) Start(context.Context) error     { return nil }
func (p *capturePipeline) Stop() error                     { return nil }
func (p *capturePipeline) Interrupt() error                { return nil }
func (p *capturePipeline) Output() <-chan pipeline.Message { return nil }
func (p *capturePipeline) Input() chan<- pipeline.Message  { return nil }
func (p *capturePipeline) Emit(msg pipeline.Message) error {
	p.mu.Lock()
	p.emitted = append(p.emitted, msg)
	p.mu.Unlock()
	select {
	case p.emitReady <- struct{}{}:
	default:
	}
	return nil
}

func TestRegistryMountSearchAndDismount(t *testing.T) {
	r := NewRegistry()
	created, err := r.Create(CreateTaskRequest{ID: "task-1", Title: "Plan Beijing trip", SubAgentID: "agent-1"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Status != StatusActive {
		t.Fatalf("status = %q", created.Status)
	}
	if !r.Mount(created.ID, "session-1") {
		t.Fatal("Mount() = false")
	}
	if !r.Mount(created.ID, "session-1") {
		t.Fatal("second Mount() = false")
	}
	got, ok := r.Get(created.ID)
	if !ok || len(got.MountedSessions) != 1 {
		t.Fatalf("mounted sessions = %#v", got)
	}
	if matches := r.Search("beijing"); len(matches) != 1 || matches[0].ID != created.ID {
		t.Fatalf("Search() = %#v", matches)
	}
	if !r.Dismount(created.ID, "session-1") {
		t.Fatal("Dismount() = false")
	}
	got, _ = r.Get(created.ID)
	if len(got.MountedSessions) != 0 {
		t.Fatalf("mounted sessions after dismount = %#v", got.MountedSessions)
	}
}

func TestRegistryForwardsSubAgentOutputToMountedSession(t *testing.T) {
	sessions := session.NewManager()
	output := &capturePipeline{emitReady: make(chan struct{}, 1)}
	sess := session.New(session.SessionMeta{})
	sess.ID = "session-1"
	sess.Pipeline = output
	sessions.Add(sess)

	r := NewRegistry(sessions)
	created, err := r.Create(CreateTaskRequest{ID: "task-1", Title: "Plan trip"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !r.Mount(created.ID, sess.ID) {
		t.Fatal("Mount() = false")
	}
	sa := agent.NewSubAgent("agent-1", created.ID, nil)
	if err := r.AttachSubAgent(created.ID, sa); err != nil {
		t.Fatalf("AttachSubAgent() error = %v", err)
	}
	sa.OutputCh <- pipeline.NewMessage(pipeline.MessageTypeData, "draft")
	sa.OutputCh <- pipeline.NewMessage(pipeline.MessageTypeFinished, nil)
	close(sa.OutputCh)

	select {
	case <-output.emitReady:
	case <-time.After(time.Second):
		t.Fatal("task output was not forwarded")
	}
	deadline := time.Now().Add(time.Second)
	for {
		got, ok := r.Get(created.ID)
		if ok && got.Status == StatusCompleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("task was not marked completed")
		}
		time.Sleep(time.Millisecond)
	}
	output.mu.Lock()
	defer output.mu.Unlock()
	if len(output.emitted) != 2 || output.emitted[0].Payload != "draft" {
		t.Fatalf("emitted = %#v", output.emitted)
	}
}
