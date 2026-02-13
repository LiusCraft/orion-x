package memory

import (
	"context"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"
	"github.com/liuscraft/orion-x/internal/logging"
)

// Summarizer 用于生成会话摘要。
type Summarizer interface {
	Summarize(ctx context.Context, turns []Turn) (string, error)
}

// SessionBuffer 保存当前 session 的对话历史。
type SessionBuffer struct {
	maxTurns      int
	summaryEveryN int
	summarizer    Summarizer

	mu         sync.Mutex
	turns      []Turn
	summary    string
	totalTurns int
}

func NewSessionBuffer(maxTurns, summaryEveryN int, summarizer Summarizer) *SessionBuffer {
	if maxTurns <= 0 {
		maxTurns = 10
	}
	if summaryEveryN < 0 {
		summaryEveryN = 0
	}
	return &SessionBuffer{
		maxTurns:      maxTurns,
		summaryEveryN: summaryEveryN,
		summarizer:    summarizer,
		turns:         make([]Turn, 0, maxTurns),
	}
}

func (b *SessionBuffer) Add(ctx context.Context, turn Turn) {
	b.mu.Lock()
	b.turns = append(b.turns, turn)
	b.totalTurns++
	if len(b.turns) > b.maxTurns {
		b.turns = b.turns[len(b.turns)-b.maxTurns:]
	}
	shouldSummarize := b.summaryEveryN > 0 && b.totalTurns%b.summaryEveryN == 0
	turnsSnapshot := append([]Turn(nil), b.turns...)
	b.mu.Unlock()

	if shouldSummarize && b.summarizer != nil {
		summary, err := b.summarizer.Summarize(ctx, turnsSnapshot)
		if err != nil {
			logging.Warnf("Memory: session summary failed: %v", err)
			return
		}
		summary = strings.TrimSpace(summary)
		if summary == "" {
			return
		}
		b.mu.Lock()
		b.summary = summary
		b.mu.Unlock()
	}
}

func (b *SessionBuffer) Messages() []*schema.Message {
	b.mu.Lock()
	defer b.mu.Unlock()

	messages := make([]*schema.Message, 0, len(b.turns)*2+1)
	if strings.TrimSpace(b.summary) != "" {
		messages = append(messages, schema.SystemMessage("近期对话摘要: "+b.summary))
	}

	for _, turn := range b.turns {
		userText := strings.TrimSpace(turn.UserText)
		if userText != "" {
			messages = append(messages, schema.UserMessage(userText))
		}
		assistantText := strings.TrimSpace(turn.AssistantText)
		if assistantText != "" {
			messages = append(messages, schema.AssistantMessage(assistantText, nil))
		}
	}
	return messages
}
