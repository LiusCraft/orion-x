package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
)

// LLMConfig 提供 LLM 连接信息。
type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

type llmSummarizer struct {
	model *openai.ChatModel
}

func (s *llmSummarizer) Summarize(ctx context.Context, turns []Turn) (string, error) {
	if len(turns) == 0 {
		return "", nil
	}
	var b strings.Builder
	for i, turn := range turns {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("用户: %s\n助手: %s", strings.TrimSpace(turn.UserText), strings.TrimSpace(turn.AssistantText)))
	}
	messages := []*schema.Message{
		schema.SystemMessage("你是对话摘要助手，请用简洁中文总结最近的对话要点，控制在100字以内。"),
		schema.UserMessage(b.String()),
	}
	resp, err := s.model.Generate(ctx, messages)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

type llmExtractor struct {
	model             *openai.ChatModel
	now               func() time.Time
	retentionDays     int
	defaultType       string
	defaultImportance int
}

func (e *llmExtractor) Extract(ctx context.Context, turn Turn) ([]MemoryItem, error) {
	content := strings.TrimSpace(turn.UserText)
	if content == "" {
		return nil, nil
	}
	prompt := "请从以下对话中提取‘长期稳定’的用户事实/偏好（不要提取临时上下文、一次性任务或隐私敏感信息）。\n输出 JSON 数组，每项包含：content, type(\"preference\"|\"fact\"|\"profile\"), importance(1-5)。如果没有可提取内容，输出空数组 []。"
	messages := []*schema.Message{
		schema.SystemMessage(prompt),
		schema.UserMessage(content),
	}
	resp, err := e.model.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}
	items, err := parseMemoryItems(resp.Content)
	if err != nil {
		return nil, err
	}
	items = filterMemoryItems(items)
	if len(items) == 0 {
		return nil, nil
	}
	if len(items) > 10 {
		items = items[:10]
	}
	now := e.now()
	for i := range items {
		items[i].CreatedAt = now
		if items[i].Type == "" {
			items[i].Type = e.defaultType
		}
		if items[i].Importance <= 0 {
			items[i].Importance = e.defaultImportance
		}
		if e.retentionDays > 0 {
			exp := now.AddDate(0, 0, e.retentionDays)
			items[i].ExpiresAt = &exp
		}
	}
	return items, nil
}

type memoryItemPayload struct {
	Content    string `json:"content"`
	Type       string `json:"type"`
	Importance int    `json:"importance"`
}

func parseMemoryItems(raw string) ([]MemoryItem, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	// 尝试解析 JSON
	var payload []memoryItemPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		items := make([]MemoryItem, 0, len(payload))
		for _, p := range payload {
			content := strings.TrimSpace(p.Content)
			if content == "" {
				continue
			}
			items = append(items, MemoryItem{
				Content:    content,
				Type:       strings.TrimSpace(p.Type),
				Importance: p.Importance,
			})
		}
		return items, nil
	}

	// 退化：按行解析
	lines := strings.Split(trimmed, "\n")
	items := make([]MemoryItem, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if line == "" {
			continue
		}
		items = append(items, MemoryItem{Content: line})
	}
	return items, nil
}

func filterMemoryItems(items []MemoryItem) []MemoryItem {
	filtered := make([]MemoryItem, 0, len(items))
	for _, item := range items {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		if looksLikeQuestion(content) {
			continue
		}
		if item.Importance > 0 && item.Importance < 3 {
			continue
		}
		item.Content = content
		filtered = append(filtered, item)
	}
	return filtered
}

func looksLikeQuestion(text string) bool {
	return strings.ContainsAny(text, "?？")
}

func newLLMModel(ctx context.Context, cfg LLMConfig) (*openai.ChatModel, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, nil
	}
	model, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		APIKey:  cfg.APIKey,
	})
	if err != nil {
		return nil, err
	}
	return model, nil
}
