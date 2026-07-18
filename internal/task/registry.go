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

// Registry owns task metadata and the background agents executing tasks.
type Registry struct {
	mu        sync.RWMutex
	tasks     map[string]*Task
	subAgents map[string]*agent.SubAgent
	sessions  *session.Manager
}

func NewRegistry(sessions ...*session.Manager) *Registry {
	var manager *session.Manager
	if len(sessions) > 0 {
		manager = sessions[0]
	}
	return &Registry{tasks: make(map[string]*Task), subAgents: make(map[string]*agent.SubAgent), sessions: manager}
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
	t := &Task{ID: req.ID, Title: req.Title, Status: StatusActive, SubAgentID: req.SubAgentID, CreatedAt: time.Now().UTC()}
	r.tasks[t.ID] = t
	return cloneTask(t), nil
}

func (r *Registry) RegisterSubAgent(sa *agent.SubAgent) {
	if r == nil || sa == nil || sa.ID == "" {
		return
	}
	r.mu.Lock()
	r.subAgents[sa.ID] = sa
	r.mu.Unlock()
}

// AttachSubAgent associates a running sub-agent with a task and fans its
// output into every mounted session's final pipeline output.
func (r *Registry) AttachSubAgent(taskID string, sa *agent.SubAgent) error {
	if r == nil || sa == nil || sa.ID == "" {
		return errors.New("task and sub-agent are required")
	}
	r.mu.Lock()
	t, ok := r.tasks[taskID]
	if !ok {
		r.mu.Unlock()
		return errors.New("task not found")
	}
	t.SubAgentID = sa.ID
	r.subAgents[sa.ID] = sa
	r.mu.Unlock()
	go r.forwardOutput(taskID, sa.OutputCh)
	return nil
}

func (r *Registry) forwardOutput(taskID string, output <-chan pipeline.Message) {
	for msg := range output {
		r.updateFromMessage(taskID, msg)
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

func (r *Registry) updateFromMessage(taskID string, msg pipeline.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tasks[taskID]
	if !ok {
		return
	}
	switch msg.Type {
	case pipeline.MessageTypeData:
		if text, ok := msg.Payload.(string); ok && strings.TrimSpace(text) != "" {
			t.Progress += text
		}
	case pipeline.MessageTypeFinished:
		t.Status = StatusCompleted
	case pipeline.MessageTypeError:
		t.Status = StatusCancelled
		if msg.Metadata.Error != nil {
			t.Progress = msg.Metadata.Error.Error()
		}
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

func (r *Registry) Get(id string) (*Task, bool) {
	r.mu.RLock()
	t, ok := r.tasks[id]
	r.mu.RUnlock()
	return cloneTask(t), ok
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
// session. start is responsible for constructing and starting the isolated
// sub-agent after a task record has been created.
func (r *Registry) ToolSpecs(sessionID string, start func(context.Context, *Task) error) []tools.Spec {
	return []tools.Spec{
		{
			Name: "CreateTask", Description: "Create a background task for a complex request.",
			Parameters: objectSchema(map[string]any{"title": map[string]any{"type": "string", "description": "Task title and objective"}}, "title"),
			Execute: func(ctx context.Context, arguments json.RawMessage) (tools.Result, error) {
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
				if start != nil {
					if err := start(ctx, task); err != nil {
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
