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
	SubAgentID      string
	MountedSessions []string
	Progress        string
	CreatedAt       time.Time
}

type CreateTaskRequest struct {
	ID         string
	Title      string
	SubAgentID string
}
