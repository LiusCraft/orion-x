package task

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// stubRun 是测试用的执行句柄：事件由测试直接推入。
type stubRun struct {
	ref    string
	events chan Event
}

func newStubRun(ref string, buffer int) *stubRun {
	return &stubRun{ref: ref, events: make(chan Event, buffer)}
}

func (r *stubRun) Events() <-chan Event            { return r.events }
func (r *stubRun) Ref() string                     { return r.ref }
func (r *stubRun) Interrupt(context.Context) error { return nil }
func (r *stubRun) Close() error                    { return nil }

// stubRunner 记录启动请求并返回预置的执行句柄。
type stubRunner struct {
	run     Run
	started *RunRequest
}

func (s *stubRunner) Start(req RunRequest) (Run, error) {
	s.started = &req
	return s.run, nil
}

type capturePipeline struct {
	mu        sync.Mutex
	emitted   []pipeline.Message
	emitReady chan struct{}
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

func TestTaskToolSpecsCreateAndMount(t *testing.T) {
	r := NewRegistry()
	run := newStubRun("run:task-1", 1)
	runner := &stubRunner{run: run}
	registry := tools.NewRegistry(r.ToolSpecs("session-1", runner)...)
	result, err := registry.Execute(context.Background(), "CreateTask", json.RawMessage(`{"title":"Research itinerary"}`))
	if err != nil {
		t.Fatalf("CreateTask error = %v", err)
	}
	if runner.started == nil || runner.started.Title != "Research itinerary" {
		t.Fatalf("runner started with %+v", runner.started)
	}
	var created Task
	if err := json.Unmarshal([]byte(result.Output), &created); err != nil {
		t.Fatalf("decode result = %v", err)
	}
	if created.RunID != "run:task-1" || created.OriginSessionID != "session-1" ||
		len(created.MountedSessions) != 1 || created.MountedSessions[0] != "session-1" {
		t.Fatalf("created task = %#v", created)
	}
	result, err = registry.Execute(context.Background(), "GetTaskProgress", json.RawMessage(`{"task_id":"`+created.ID+`"}`))
	if err != nil {
		t.Fatalf("GetTaskProgress error = %v", err)
	}
	if result.Output == "" {
		t.Fatal("GetTaskProgress returned an empty result")
	}
	close(run.events)
}

func TestRegistryMountSearchAndDismount(t *testing.T) {
	r := NewRegistry()
	created, err := r.Create(CreateTaskRequest{ID: "task-1", Title: "Plan Beijing trip"})
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

func TestRegistryForwardsRunEventsToMountedSession(t *testing.T) {
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
	run := newStubRun("run:task-1", 4)
	if err := r.Attach(created.ID, run); err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	run.events <- Event{Type: EventText, Text: "draft"}
	run.events <- Event{Type: EventToolUse, Tool: &ToolUse{Name: "web_search"}}
	run.events <- Event{Type: EventDone, Usage: &agent.Usage{InputTokens: 10, OutputTokens: 5, Steps: 1}}
	close(run.events)

	select {
	case <-output.emitReady:
	case <-time.After(time.Second):
		t.Fatal("task output was not forwarded")
	}
	waitForStatus(t, r, created.ID, StatusCompleted)

	output.mu.Lock()
	defer output.mu.Unlock()
	if len(output.emitted) != 2 || output.emitted[0].Payload != "draft" {
		t.Fatalf("emitted = %#v", output.emitted)
	}
	usage, ok := agent.UsageFromMetadata(output.emitted[1].Metadata)
	if !ok || usage.Steps != 1 || usage.InputTokens != 10 {
		t.Fatalf("finished usage = %+v, ok = %t", usage, ok)
	}
}

func TestRegistryMarksTaskCancelledOnRunError(t *testing.T) {
	r := NewRegistry()
	created, err := r.Create(CreateTaskRequest{ID: "task-1", Title: "doomed"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	run := newStubRun("run:task-1", 1)
	if err := r.Attach(created.ID, run); err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	run.events <- Event{Type: EventError, Err: errors.New("boom")}
	close(run.events)

	waitForStatus(t, r, created.ID, StatusCancelled)
	got, _ := r.Get(created.ID)
	if got.Progress != "boom" {
		t.Fatalf("progress = %q", got.Progress)
	}
}

func waitForStatus(t *testing.T, r *Registry, taskID string, want Status) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		got, ok := r.Get(taskID)
		if ok && got.Status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %s did not reach status %q", taskID, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestRegistryForwardsRunErrorUsage 验证执行失败时，已发生的用量也会写进
// pipeline 消息的元数据：错误路径同样要计费。
func TestRegistryForwardsRunErrorUsage(t *testing.T) {
	sessions := session.NewManager()
	output := &capturePipeline{emitReady: make(chan struct{}, 1)}
	sess := session.New(session.SessionMeta{})
	sess.ID = "session-1"
	sess.Pipeline = output
	sessions.Add(sess)

	r := NewRegistry(sessions)
	created, err := r.Create(CreateTaskRequest{ID: "task-1", Title: "doomed"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !r.Mount(created.ID, sess.ID) {
		t.Fatal("Mount() = false")
	}
	run := newStubRun("run:task-1", 1)
	if err := r.Attach(created.ID, run); err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	run.events <- Event{Type: EventError, Err: errors.New("boom"), Usage: &agent.Usage{InputTokens: 7, Steps: 1}}
	close(run.events)

	select {
	case <-output.emitReady:
	case <-time.After(time.Second):
		t.Fatal("task error was not forwarded")
	}
	waitForStatus(t, r, created.ID, StatusCancelled)

	output.mu.Lock()
	defer output.mu.Unlock()
	if len(output.emitted) != 1 {
		t.Fatalf("emitted = %#v", output.emitted)
	}
	usage, ok := agent.UsageFromMetadata(output.emitted[0].Metadata)
	if !ok || usage.InputTokens != 7 || usage.Steps != 1 {
		t.Fatalf("error usage = %+v, ok = %t", usage, ok)
	}
}

// onlyMessage 返回管道收到的唯一一条消息，用来断言扇出内容。
func onlyMessage(t *testing.T, p *capturePipeline) pipeline.Message {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.emitted) != 1 {
		t.Fatalf("emitted %d messages, want 1", len(p.emitted))
	}
	return p.emitted[0]
}

// TestRegistryBillsUsageOnceAcrossMountedSessions 验证同一任务被多个会话挂载时，
// 用量只随一条终态消息记账：计费是会话级的，把同一条带 llm_usage 的消息扇出给
// 每个挂载会话，会把同一批 token 重复计入（甚至记到别的账户）。
func TestRegistryBillsUsageOnceAcrossMountedSessions(t *testing.T) {
	sessions := session.NewManager()
	pipelines := map[string]*capturePipeline{}
	for _, id := range []string{"session-1", "session-2"} {
		output := &capturePipeline{emitReady: make(chan struct{}, 1)}
		pipelines[id] = output
		sess := session.New(session.SessionMeta{})
		sess.ID = id
		sess.Pipeline = output
		sessions.Add(sess)
	}

	r := NewRegistry(sessions)
	created, err := r.Create(CreateTaskRequest{ID: "task-1", Title: "Plan trip", OriginSessionID: "session-1"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !r.Mount(created.ID, "session-1") || !r.Mount(created.ID, "session-2") {
		t.Fatal("Mount() = false")
	}
	run := newStubRun("run:task-1", 1)
	if err := r.Attach(created.ID, run); err != nil {
		t.Fatalf("Attach() error = %v", err)
	}
	run.events <- Event{Type: EventDone, Usage: &agent.Usage{InputTokens: 10, OutputTokens: 5, Steps: 1}}
	close(run.events)

	for id, output := range pipelines {
		select {
		case <-output.emitReady:
		case <-time.After(time.Second):
			t.Fatalf("%s did not receive the terminal message", id)
		}
	}

	origin := onlyMessage(t, pipelines["session-1"])
	if usage, ok := agent.UsageFromMetadata(origin.Metadata); !ok || usage.InputTokens != 10 {
		t.Errorf("origin usage = %+v, ok = %t", usage, ok)
	}
	other := onlyMessage(t, pipelines["session-2"])
	if other.Type != pipeline.MessageTypeFinished {
		t.Errorf("non-origin message type = %v, want finished", other.Type)
	}
	if usage, ok := agent.UsageFromMetadata(other.Metadata); ok {
		t.Errorf("non-origin session got usage = %+v, want none", usage)
	}
}
