package task

import "time"

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
	StatusCancelled Status = "cancelled"
)

type Task struct {
	ID              string
	Title           string
	Status          Status
	RunID           string
	MountedSessions []string
	Progress        string
	CreatedAt       time.Time
	// OriginSessionID 是创建任务的会话。任务用量只在这一个会话上计费——
	// 其它会话可以挂载任务、接收进度，但不能重复记账同一笔 token。
	OriginSessionID string
}

type CreateTaskRequest struct {
	ID    string
	Title string
	// OriginSessionID 是发起任务的会话；为空时用量退回到第一个挂载会话。
	OriginSessionID string
}
