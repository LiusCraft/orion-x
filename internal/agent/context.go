package agent

import (
	"context"

	"github.com/cloudwego/eino/schema"
	"github.com/liuscraft/orion-x/internal/logging"
)

func (v *Agent) buildMessages(ctx context.Context, input string) []*schema.Message {
	if v.memorySvc != nil {
		messages, err := v.memorySvc.BuildContextMessages(ctx, input)
		if err != nil {
			logging.Warnf("Agent: build memory context failed: %v", err)
		}
		if len(messages) > 0 {
			return messages
		}
	}
	return []*schema.Message{
		schema.SystemMessage(defaultSystemPrompt),
		schema.UserMessage(input),
	}
}
