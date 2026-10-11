package task

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

// Registry owns task metadata and the background executions dispatched for tasks.
type Registry struct {
	mu       sync.RWMutex
	tasks    map[string]*Task
	sessions *session.Manager
}

func NewRegistry(sessions ...*session.Manager) *Registry {
	var manager *session.Manager
	if len(sessions) > 0 {
		manager = sessions[0]
	}
	return &Registry{tasks: make(map[string]*Task), sessions: manager}
}

func (r *Registry) Create(req CreateTaskRequest) (*Task, error) {
	if r == nil || strings.TrimSpace(req.ID) == "" || strings.TrimSpace(req.Title) == "" {
		return nil, errors.New("task id and title are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tasks[req.ID]; exists {
		return nil, errors.New("task already exists")
	}
	t := &Task{ID: req.ID, Title: req.Title, Status: StatusActive, CreatedAt: time.Now().UTC()}
	r.tasks[t.ID] = t
	return cloneTask(t), nil
}

// Attach binds a running execution to a task and fans its events into every
// mounted session's pipeline.
func (r *Registry) Attach(taskID string, run Run) error {
	if r == nil || run == nil {
		return errors.New("task and run are required")
	}
	r.mu.Lock()
	t, ok := r.tasks[taskID]
	if !ok {
		r.mu.Unlock()
		return errors.New("task not found")
	}
	t.RunID = run.Ref()
	r.mu.Unlock()
	go r.forwardOutput(taskID, run)
	return nil
}

func (r *Registry) forwardOutput(taskID string, run Run) {
	for event := range run.Events() {
		r.updateFromEvent(taskID, event)
		msg, ok := eventMessage(taskID, event)
		if !ok {
			continue
		}
		for _, sessionID := range r.mountedSessionIDs(taskID) {
			if r.sessions == nil {
				continue
			}
			sess, ok := r.sessions.Get(sessionID)
			if !ok || sess.Pipeline == nil {
				continue
			}
			_ = sess.Pipeline.Emit(msg)
		}
	}
}

func (r *Registry) updateFromEvent(taskID string, event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[taskID]
	if !ok {
		return
	}
	switch event.Type {
	case EventText:
		if strings.TrimSpace(event.Text) != "" {
			t.Progress += event.Text
		}
	case EventDone:
		t.Status = StatusCompleted
	case EventError:
		t.Status = StatusCancelled
		if event.Err != nil {
			t.Progress = event.Err.Error()
		}
	}
}

// eventMessage 把执行事件转换成会话 pipeline 消息。source 保持
// "sub_agent" 以免影响各渠道对已有输出流的路由。
func eventMessage(taskID string, event Event) (pipeline.Message, bool) {
	switch event.Type {
	case EventText:
		msg := pipeline.NewMessage(pipeline.MessageTypeData, event.Text)
		msg.Metadata.Extra = map[string]interface{}{"source": "sub_agent", "task_id": taskID}
		return msg, true
	case EventDone:
		msg := pipeline.NewMessage(pipeline.MessageTypeFinished, nil)
		msg.Metadata.Extra = map[string]interface{}{"source": "sub_agent", "task_id": taskID}
		if event.Usage != nil {
			msg.Metadata.Extra[agent.UsageMetadataKey] = *event.Usage
		}
		return msg, true
	case EventError:
		msg := pipeline.NewMessage(pipeline.MessageTypeError, nil).WithError(event.Err)
		msg.Metadata.Extra = map[string]interface{}{"source": "sub_agent", "task_id": taskID}
		return msg, true
	default:
		return pipeline.Message{}, false
	}
}

func (r *Registry) mountedSessionIDs(taskID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[taskID]
	if !ok {
		return nil
	}
	return append([]string(nil), t.MountedSessions...)
}

// Get returns a snapshot copy of a task. The clone must happen while the read
// lock is held: the forwarding goroutine mutates the stored task under the
// write lock, so copying it after unlocking would race with that writer.
func (r *Registry) Get(id string) (*Task, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, false
	}
	return cloneTask(t), true
}

func (r *Registry) Mount(taskID, sessionID string) bool { return r.setMount(taskID, sessionID, true) }
func (r *Registry) Dismount(taskID, sessionID string) bool {
	return r.setMount(taskID, sessionID, false)
}

func (r *Registry) setMount(taskID, sessionID string, mount bool) bool {
	if strings.TrimSpace(sessionID) == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[taskID]
	if !ok {
		return false
	}
	for i, id := range t.MountedSessions {
		if id != sessionID {
			continue
		}
		if !mount {
			t.MountedSessions = append(t.MountedSessions[:i], t.MountedSessions[i+1:]...)
		}
		return true
	}
	if mount {
		t.MountedSessions = append(t.MountedSessions, sessionID)
	}
	return true
}

func (r *Registry) Search(query string) []*Task {
	query = strings.ToLower(strings.TrimSpace(query))
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Task, 0)
	for _, t := range r.tasks {
		if query == "" || strings.Contains(strings.ToLower(t.Title), query) || strings.Contains(strings.ToLower(t.Progress), query) {
			result = append(result, cloneTask(t))
		}
	}
	return result
}

func cloneTask(t *Task) *Task {
	if t == nil {
		return nil
	}
	clone := *t
	clone.MountedSessions = append([]string(nil), t.MountedSessions...)
	return &clone
}

// ToolSpecs returns the task operations available to a primary agent in one
// session. runner is responsible for starting the background execution after a
// task record has been created; nil keeps tasks metadata-only (useful in tests).
func (r *Registry) ToolSpecs(sessionID string, runner Runner) []tools.Spec {
	return []tools.Spec{
		{
			Name: "CreateTask", Description: "Create a background task for a complex request.",
			Parameters: objectSchema(map[string]any{"title": map[string]any{"type": "string", "description": "Task title and objective"}}, "title"),
			Execute: func(_ context.Context, arguments json.RawMessage) (tools.Result, error) {
				var req struct {
					Title string `json:"title"`
				}
				if err := json.Unmarshal(arguments, &req); err != nil {
					return tools.Result{}, err
				}
				task, err := r.Create(CreateTaskRequest{ID: newID(), Title: strings.TrimSpace(req.Title)})
				if err != nil {
					return tools.Result{}, err
				}
				if !r.Mount(task.ID, sessionID) {
					return tools.Result{}, errors.New("mount created task")
				}
				if runner != nil {
					run, err := runner.Start(RunRequest{TaskID: task.ID, Title: task.Title})
					if err != nil {
						return tools.Result{}, fmt.Errorf("start task: %w", err)
					}
					if err := r.Attach(task.ID, run); err != nil {
						return tools.Result{}, err
					}
				}
				mounted, _ := r.Get(task.ID)
				return jsonResult(mounted)
			},
		},
		{
			Name: "SearchTasks", Description: "Search background tasks by title or progress.",
			Parameters: objectSchema(map[string]any{"query": map[string]any{"type": "string"}}, "query"),
			Execute: func(_ context.Context, arguments json.RawMessage) (tools.Result, error) {
				var req struct {
					Query string `json:"query"`
				}
				if err := json.Unmarshal(arguments, &req); err != nil {
					return tools.Result{}, err
				}
				return jsonResult(r.Search(req.Query))
			},
		},
		{
			Name: "MountTask", Description: "Mount a task's progress into this conversation.",
			Parameters: objectSchema(map[string]any{"task_id": map[string]any{"type": "string"}}, "task_id"),
			Execute: func(_ context.Context, arguments json.RawMessage) (tools.Result, error) {
				var req struct {
					TaskID string `json:"task_id"`
				}
				if err := json.Unmarshal(arguments, &req); err != nil {
					return tools.Result{}, err
				}
				if !r.Mount(req.TaskID, sessionID) {
					return tools.Result{}, errors.New("task not found")
				}
				task, _ := r.Get(req.TaskID)
				return jsonResult(task)
			},
		},
		{
			Name: "DismountTask", Description: "Stop receiving a task's progress in this conversation.",
			Parameters: objectSchema(map[string]any{"task_id": map[string]any{"type": "string"}}, "task_id"),
			Execute: func(_ context.Context, arguments json.RawMessage) (tools.Result, error) {
				var req struct {
					TaskID string `json:"task_id"`
				}
				if err := json.Unmarshal(arguments, &req); err != nil {
					return tools.Result{}, err
				}
				if !r.Dismount(req.TaskID, sessionID) {
					return tools.Result{}, errors.New("task not found")
				}
				task, _ := r.Get(req.TaskID)
				return jsonResult(task)
			},
		},
		{
			Name: "GetTaskProgress", Description: "Get the status and current output of a task.",
			Parameters: objectSchema(map[string]any{"task_id": map[string]any{"type": "string"}}, "task_id"),
			Execute: func(_ context.Context, arguments json.RawMessage) (tools.Result, error) {
				var req struct {
					TaskID string `json:"task_id"`
				}
				if err := json.Unmarshal(arguments, &req); err != nil {
					return tools.Result{}, err
				}
				task, ok := r.Get(req.TaskID)
				if !ok {
					return tools.Result{}, errors.New("task not found")
				}
				return jsonResult(task)
			},
		},
	}
}

func objectSchema(properties map[string]any, required string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": []string{required}}
}

func jsonResult(value any) (tools.Result, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return tools.Result{}, err
	}
	return tools.Result{Output: string(data)}, nil
}

func newID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("task_%d", time.Now().UnixNano())
	}
	return "task_" + hex.EncodeToString(buf)
}
