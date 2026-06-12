# Session 包实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 `internal/session` 包，提供聊天记录管理能力。

**Architecture:** 纯数据层包，无外部依赖（仅依赖 `crypto/rand`、`time` 和 `internal/llm`）。Session 是消息容器，提供 Add/Pop/ToLLMMessages 等方法。

**Tech Stack:** Go 标准库 + `internal/llm` 类型转换

---

### Task 1: 类型定义和 New 构造函数

**Files:**
- Create: `internal/session/session.go`

- [ ] **Step 1: 创建类型定义和 New 构造函数**

```go
package session

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
)

// Role 消息角色
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ToolCall 工具调用
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Message 单条聊天消息
type Message struct {
	ID         string     `json:"id"`
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Status     string     `json:"status,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// SessionMeta 会话元数据
type SessionMeta struct {
	UserID    string    `json:"user_id"`
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}

// Session 一段聊天记录
type Session struct {
	ID        string      `json:"id"`
	Meta      SessionMeta `json:"meta"`
	Messages  []Message   `json:"messages"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// New 创建新会话
func New(meta SessionMeta) *Session {
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	return &Session{
		ID:        newID("sess"),
		Meta:      meta,
		Messages:  make([]Message, 0, 16),
		UpdatedAt: now,
	}
}

func newID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "_fallback"
	}
	return prefix + "_" + hex.EncodeToString(buf)
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./internal/session/
```

---

### Task 2: Add 方法

**Files:**
- Modify: `internal/session/session.go`

- [ ] **Step 1: 实现 Add 方法**

在 `session.go` 文件中 `newID` 函数之后添加：

```go
// Add 添加消息
func (s *Session) Add(msg Message) {
	if msg.ID == "" {
		msg.ID = newID("msg")
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	s.Messages = append(s.Messages, msg)
	s.UpdatedAt = msg.CreatedAt
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./internal/session/
```

---

### Task 3: Add 方法测试

**Files:**
- Create: `internal/session/session_test.go`

- [ ] **Step 1: 编写 TestNew 和 TestAdd 测试**

```go
package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	meta := SessionMeta{
		UserID: "user-1",
		Model:  "gpt-4",
	}
	sess := New(meta)

	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if !strings.HasPrefix(sess.ID, "sess_") {
		t.Errorf("ID should start with 'sess_', got %q", sess.ID)
	}
	if sess.Meta.UserID != "user-1" {
		t.Errorf("UserID: got %q, want %q", sess.Meta.UserID, "user-1")
	}
	if sess.Meta.Model != "gpt-4" {
		t.Errorf("Model: got %q, want %q", sess.Meta.Model, "gpt-4")
	}
	if len(sess.Messages) != 0 {
		t.Fatalf("expected empty messages, got %d", len(sess.Messages))
	}
	if sess.Meta.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if sess.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestNewDefaultCreatedAt(t *testing.T) {
	meta := SessionMeta{
		UserID: "u1",
		Model:  "m1",
	}
	sess := New(meta)
	if sess.Meta.CreatedAt.IsZero() {
		t.Error("CreatedAt should be auto-filled if zero")
	}
}

func TestAdd(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})

	msg := Message{Role: RoleUser, Content: "hello"}
	sess.Add(msg)

	if len(sess.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sess.Messages))
	}
	got := sess.Messages[0]
	if got.Role != RoleUser {
		t.Errorf("Role: got %v, want %v", got.Role, RoleUser)
	}
	if got.Content != "hello" {
		t.Errorf("Content: got %q, want %q", got.Content, "hello")
	}
	if got.ID == "" {
		t.Error("ID should be auto-generated")
	}
	if !strings.HasPrefix(got.ID, "msg_") {
		t.Errorf("ID should start with 'msg_', got %q", got.ID)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestAddPreservesID(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})

	msg := Message{ID: "my-custom-id", Role: RoleUser, Content: "hi"}
	sess.Add(msg)

	if sess.Messages[0].ID != "my-custom-id" {
		t.Errorf("ID should be preserved, got %q", sess.Messages[0].ID)
	}
}

func TestAddUpdatesUpdatedAt(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	before := sess.UpdatedAt

	time.Sleep(time.Millisecond)
	msg := Message{Role: RoleUser, Content: "hi"}
	sess.Add(msg)

	if !sess.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt not updated: before=%v, after=%v", before, sess.UpdatedAt)
	}
}

func TestMessageIDUnique(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleUser, Content: "b"})
	sess.Add(Message{Role: RoleUser, Content: "c"})

	ids := map[string]bool{
		sess.Messages[0].ID: true,
		sess.Messages[1].ID: true,
		sess.Messages[2].ID: true,
	}
	if len(ids) != 3 {
		t.Errorf("IDs should be unique")
	}
}

func TestSessionJSONRoundTrip(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "gpt-4"})
	sess.Add(Message{Role: RoleUser, Content: "hello"})
	sess.Add(Message{Role: RoleAssistant, Content: "hi", ToolCalls: []ToolCall{
		{ID: "call_1", Name: "greet", Arguments: `{"name":"world"}`},
	}})
	sess.Add(Message{Role: RoleTool, Content: "result", ToolCallID: "call_1", Status: "completed"})

	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored Session
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.ID != sess.ID {
		t.Errorf("ID mismatch")
	}
	if restored.Meta.UserID != sess.Meta.UserID {
		t.Errorf("UserID mismatch")
	}
	if len(restored.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(restored.Messages))
	}
	if restored.Messages[1].ToolCalls[0].Name != "greet" {
		t.Errorf("ToolCall name mismatch")
	}
	if restored.Messages[2].ToolCallID != "call_1" {
		t.Errorf("ToolCallID mismatch")
	}
	if restored.Messages[2].Status != "completed" {
		t.Errorf("Status mismatch")
	}
}
```

- [ ] **Step 2: 运行测试验证**

```bash
go test ./internal/session/ -v -run "TestNew|TestAdd|TestMessageID|TestSessionJSON"
```

Expected: PASS

---

### Task 4: Pop 和 PopN 方法

**Files:**
- Modify: `internal/session/session.go`

- [ ] **Step 1: 实现 Pop 和 PopN**

在 `Add` 方法之后添加：

```go
// Pop 移除并返回最后一条消息
func (s *Session) Pop() (Message, bool) {
	if len(s.Messages) == 0 {
		return Message{}, false
	}
	last := s.Messages[len(s.Messages)-1]
	s.Messages = s.Messages[:len(s.Messages)-1]
	s.UpdatedAt = time.Now().UTC()
	return last, true
}

// PopN 移除最后 n 条消息，按原序返回被移除的消息
func (s *Session) PopN(n int) []Message {
	if n <= 0 {
		return nil
	}
	if n > len(s.Messages) {
		n = len(s.Messages)
	}
	idx := len(s.Messages) - n
	removed := make([]Message, n)
	copy(removed, s.Messages[idx:])
	s.Messages = s.Messages[:idx]
	s.UpdatedAt = time.Now().UTC()
	return removed
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./internal/session/
```

---

### Task 5: Pop/PopN 测试

**Files:**
- Modify: `internal/session/session_test.go`

- [ ] **Step 1: 添加 Pop/PopN 测试**

在测试文件末尾添加：

```go
func TestPop(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})

	_, ok := sess.Pop()
	if ok {
		t.Fatal("expected false for empty session")
	}

	sess.Add(Message{Role: RoleUser, Content: "first"})
	sess.Add(Message{Role: RoleUser, Content: "second"})

	msg, ok := sess.Pop()
	if !ok {
		t.Fatal("expected true for non-empty session")
	}
	if msg.Content != "second" {
		t.Errorf("Content: got %q, want 'second'", msg.Content)
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("expected 1 remaining message, got %d", len(sess.Messages))
	}
	if sess.Messages[0].Content != "first" {
		t.Errorf("remaining message: got %q, want 'first'", sess.Messages[0].Content)
	}
}

func TestPopN(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})

	// PopN on empty
	removed := sess.PopN(2)
	if len(removed) != 0 {
		t.Fatalf("expected 0 removed, got %d", len(removed))
	}

	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleAssistant, Content: "b"})
	sess.Add(Message{Role: RoleTool, Content: "c"})
	sess.Add(Message{Role: RoleAssistant, Content: "d"})

	// PopN more than length
	removed = sess.PopN(10)
	if len(removed) != 4 {
		t.Fatalf("expected 4 removed, got %d", len(removed))
	}
	if len(sess.Messages) != 0 {
		t.Fatalf("expected 0 remaining, got %d", len(sess.Messages))
	}
	if removed[0].Content != "a" {
		t.Errorf("removed[0]: got %q, want 'a'", removed[0].Content)
	}
	if removed[3].Content != "d" {
		t.Errorf("removed[3]: got %q, want 'd'", removed[3].Content)
	}
}

func TestPopNPartial(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleAssistant, Content: "b"})
	sess.Add(Message{Role: RoleTool, Content: "c"})

	removed := sess.PopN(2)
	if len(removed) != 2 {
		t.Fatalf("expected 2 removed, got %d", len(removed))
	}
	if removed[0].Content != "b" || removed[1].Content != "c" {
		t.Errorf("wrong removed: %v %v", removed[0].Content, removed[1].Content)
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(sess.Messages))
	}
	if sess.Messages[0].Content != "a" {
		t.Errorf("remaining: got %q, want 'a'", sess.Messages[0].Content)
	}
}

func TestPopNZero(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})

	removed := sess.PopN(0)
	if len(removed) != 0 {
		t.Errorf("PopN(0) should return nil, got %v", removed)
	}
	if len(sess.Messages) != 1 {
		t.Errorf("PopN(0) should not modify messages")
	}
}

func TestPopUpdatesUpdatedAt(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "hi"})

	before := sess.UpdatedAt
	time.Sleep(time.Millisecond)
	sess.Pop()

	if !sess.UpdatedAt.After(before) {
		t.Error("UpdatedAt should be updated after Pop")
	}
}
```

- [ ] **Step 2: 运行 Pop/PopN 测试**

```bash
go test ./internal/session/ -v -run "TestPop"
```

Expected: PASS

---

### Task 6: ToLLMMessages 和 LastN 方法

**Files:**
- Modify: `internal/session/session.go`

- [ ] **Step 1: 实现 ToLLMMessages 和 LastN**

在 `PopN` 函数之后添加：

```go
// ToLLMMessages 转换为 LLM 请求格式
func (s *Session) ToLLMMessages() []llm.Message {
	result := make([]llm.Message, len(s.Messages))
	for i, msg := range s.Messages {
		result[i] = llm.Message{
			Role:       string(msg.Role),
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		if len(msg.ToolCalls) > 0 {
			result[i].ToolCalls = make([]llm.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				result[i].ToolCalls[j] = llm.ToolCall{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: tc.Arguments,
				}
			}
		}
	}
	return result
}

// LastN 获取最近 n 条消息（只读）
func (s *Session) LastN(n int) []Message {
	if n <= 0 || len(s.Messages) == 0 {
		return nil
	}
	if n > len(s.Messages) {
		n = len(s.Messages)
	}
	start := len(s.Messages) - n
	return s.Messages[start:]
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./internal/session/
```

---

### Task 7: ToLLMMessages 和 LastN 测试

**Files:**
- Modify: `internal/session/session_test.go`

- [ ] **Step 1: 添加 ToLLMMessages 和 LastN 测试**

在测试文件末尾添加：

```go
func TestToLLMMessages(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "hello"})
	sess.Add(Message{Role: RoleAssistant, Content: "hi", ToolCalls: []ToolCall{
		{ID: "call_1", Name: "grep", Arguments: `{"a":"b"}`},
	}})
	sess.Add(Message{Role: RoleTool, Content: "result", ToolCallID: "call_1", Status: "completed"})

	result := sess.ToLLMMessages()
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// user message
	if result[0].Role != "user" {
		t.Errorf("Role[0]: got %q, want 'user'", result[0].Role)
	}
	if result[0].Content != "hello" {
		t.Errorf("Content[0]: got %q", result[0].Content)
	}

	// assistant with tool calls
	if result[1].Role != "assistant" {
		t.Errorf("Role[1]: got %q", result[1].Role)
	}
	if len(result[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result[1].ToolCalls))
	}
	if result[1].ToolCalls[0].Name != "grep" {
		t.Errorf("ToolCalls[0].Name: got %q", result[1].ToolCalls[0].Name)
	}

	// tool message
	if result[2].Role != "tool" {
		t.Errorf("Role[2]: got %q, want 'tool'", result[2].Role)
	}
	if result[2].ToolCallID != "call_1" {
		t.Errorf("ToolCallID: got %q", result[2].ToolCallID)
	}
	if result[2].Content != "result" {
		t.Errorf("Content[2]: got %q", result[2].Content)
	}
}

func TestToLLMMessagesEmpty(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	result := sess.ToLLMMessages()
	if len(result) != 0 {
		t.Fatalf("expected empty, got %d", len(result))
	}
}

func TestToLLMMessagesNoToolCalls(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleAssistant, Content: "plain response"})

	result := sess.ToLLMMessages()
	if result[0].ToolCalls != nil {
		t.Errorf("expected nil ToolCalls, got %v", result[0].ToolCalls)
	}
}

func TestLastN(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleAssistant, Content: "b"})
	sess.Add(Message{Role: RoleUser, Content: "c"})
	sess.Add(Message{Role: RoleAssistant, Content: "d"})

	got := sess.LastN(2)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Content != "c" || got[1].Content != "d" {
		t.Errorf("got %q %q, want 'c' 'd'", got[0].Content, got[1].Content)
	}
}

func TestLastNMoreThanLength(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})
	sess.Add(Message{Role: RoleAssistant, Content: "b"})

	got := sess.LastN(10)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestLastNZero(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "a"})

	got := sess.LastN(0)
	if got != nil {
		t.Errorf("LastN(0) should return nil, got %v", got)
	}
}

func TestLastNEmpty(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	got := sess.LastN(3)
	if got != nil {
		t.Errorf("LastN on empty should return nil, got %v", got)
	}
}
```

- [ ] **Step 2: 运行 ToLLMMessages 和 LastN 测试**

```bash
go test ./internal/session/ -v -run "TestToLLM|TestLastN"
```

Expected: PASS

---

### Task 8: Clone 方法

**Files:**
- Modify: `internal/session/session.go`

- [ ] **Step 1: 实现 Clone**

在 `LastN` 函数之后添加：

```go
// Clone 深拷贝会话
func (s *Session) Clone() *Session {
	cloned := *s

	if s.Messages != nil {
		cloned.Messages = make([]Message, len(s.Messages))
		copy(cloned.Messages, s.Messages)
		for i := range cloned.Messages {
			if len(s.Messages[i].ToolCalls) > 0 {
				cloned.Messages[i].ToolCalls = make([]ToolCall, len(s.Messages[i].ToolCalls))
				copy(cloned.Messages[i].ToolCalls, s.Messages[i].ToolCalls)
			}
		}
	}
	return &cloned
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./internal/session/
```

---

### Task 9: Clone 测试

**Files:**
- Modify: `internal/session/session_test.go`

- [ ] **Step 1: 添加 Clone 测试**

在测试文件末尾添加：

```go
func TestClone(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleUser, Content: "hello"})
	sess.Add(Message{Role: RoleAssistant, Content: "hi", ToolCalls: []ToolCall{
		{ID: "c1", Name: "foo", Arguments: `{}`},
	}})

	cloned := sess.Clone()
	if cloned.ID != sess.ID {
		t.Errorf("ID mismatch: %q vs %q", cloned.ID, sess.ID)
	}
	if len(cloned.Messages) != len(sess.Messages) {
		t.Fatalf("message count mismatch")
	}

	// 修改克隆不应影响原会话
	cloned.Messages[0].Content = "modified"
	if sess.Messages[0].Content == "modified" {
		t.Error("clone should not share message slice")
	}
}

func TestCloneToolCallsDeepCopy(t *testing.T) {
	sess := New(SessionMeta{UserID: "u1", Model: "m1"})
	sess.Add(Message{Role: RoleAssistant, Content: "hi", ToolCalls: []ToolCall{
		{ID: "c1", Name: "foo", Arguments: `{}`},
	}})

	cloned := sess.Clone()
	cloned.Messages[0].ToolCalls[0].Name = "modified"

	if sess.Messages[0].ToolCalls[0].Name == "modified" {
		t.Error("clone should deep copy ToolCalls")
	}
}

func TestCloneNilMessages(t *testing.T) {
	sess := &Session{ID: "test", Messages: nil}
	cloned := sess.Clone()
	if cloned.ID != "test" {
		t.Errorf("ID mismatch")
	}
	if cloned.Messages != nil {
		t.Errorf("expected nil Messages")
	}
}
```

- [ ] **Step 2: 运行 Clone 测试**

```bash
go test ./internal/session/ -v -run "TestClone"
```

Expected: PASS

---

### Task 10: 最终验证

**Files:**
- (全部已创建/修改)

- [ ] **Step 1: 运行全部测试**

```bash
go test ./internal/session/ -v -count=1
```

Expected: ALL PASS

- [ ] **Step 2: 运行 vet 检查**

```bash
go vet ./internal/session/
```

Expected: 无输出

- [ ] **Step 3: 提交**

```bash
git add internal/session/
git commit -m "feat: add session package for chat history management"
```
