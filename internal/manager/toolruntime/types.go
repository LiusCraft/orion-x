package toolruntime

import (
	"context"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/toolentitlement"
)

type ToolDescriptor struct {
	Name        string
	Description string
	InputSchema any
}

type ToolCallResult struct {
	ToolName string
	Result   any
}

type EntitlementReader interface {
	GetRepoEntry(ctx context.Context, userID, entitlementID uuid.UUID) (toolentitlement.RepoEntry, error)
}
