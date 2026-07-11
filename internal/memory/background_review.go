package memory

import (
	"context"
	"strings"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
)

const reviewPrompt = `回顾上面的对话，考虑是否需要保存记忆。

重点关注：
1. 用户是否透露了关于他们自己的信息——他们的性格、愿望、偏好、或个人细节值得记住？
2. 用户是否表达了对你的行为方式、工作风格、交流方式的期望？

如果有什么值得保存的，格式如下：
SAVE memory: <内容>
SAVE user: <内容>

如果没有值得保存的，就说：无需保存。
不要重复用户已经明确要求删除或不要记住的内容。`

type ReviewConfig struct {
	Enabled bool   // 是否启用
	Model   string // 自省模型 ID（空=用主模型）
	APIKey  string
	BaseURL string
}

type BackgroundReview struct {
	curated *CuratedStore
	config  ReviewConfig
	client  llm.Client
}

func NewBackgroundReview(curated *CuratedStore, config ReviewConfig, client llm.Client) *BackgroundReview {
	return &BackgroundReview{curated: curated, config: config, client: client}
}

// Spawn fires a non-blocking review. snapshot contains recent turns + existing memory.
func (r *BackgroundReview) Spawn(ctx context.Context, recentTurns []Turn, existingSnapshot string) {
	if !r.config.Enabled || r.client == nil {
		return
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				logging.Errorf("BackgroundReview: panic: %v", rec)
			}
		}()

		// Build review context from recent turns
		var b strings.Builder
		b.WriteString("当前记忆：\n")
		if existingSnapshot != "" {
			b.WriteString(existingSnapshot)
			b.WriteString("\n\n")
		}
		b.WriteString("最近对话：\n")
		for i, t := range recentTurns {
			if i > 10 {
				b.WriteString("...（更多历史已省略）\n")
				break
			}
			b.WriteString("用户: " + strings.TrimSpace(t.UserText) + "\n")
			b.WriteString("助手: " + strings.TrimSpace(t.AssistantText) + "\n")
		}

		reviewCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		resp, err := r.client.ChatSync(reviewCtx, llm.Request{
			Messages: []llm.Message{
				{Role: "system", Content: reviewPrompt},
				{Role: "user", Content: b.String()},
			},
		})
		if err != nil {
			logging.Warnf("BackgroundReview: LLM call failed: %v", err)
			return
		}

		respText := strings.TrimSpace(resp.Content)
		if respText == "" || respText == "无需保存" {
			return
		}

		// Parse SAVE memory/user directives
		for _, line := range strings.Split(respText, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "SAVE memory:") {
				content := strings.TrimSpace(strings.TrimPrefix(line, "SAVE memory:"))
				if content != "" {
					r.curated.Add("memory", content)
				}
			} else if strings.HasPrefix(line, "SAVE user:") {
				content := strings.TrimSpace(strings.TrimPrefix(line, "SAVE user:"))
				if content != "" {
					r.curated.Add("user", content)
				}
			}
		}
	}()
}
