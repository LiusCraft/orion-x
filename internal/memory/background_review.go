package memory

import (
	"context"
	"strings"

	"github.com/liuscraft/orion-x/internal/logging"
)

// ponytail: reviewPrompt defined here when LLM integration is wired up
type ReviewConfig struct {
	Enabled bool   // 是否启用
	Model   string // 自省模型 ID（空=用主模型）
	APIKey  string
	BaseURL string
}

type BackgroundReview struct {
	curated *CuratedStore
	config  ReviewConfig
}

func NewBackgroundReview(curated *CuratedStore, config ReviewConfig) *BackgroundReview {
	return &BackgroundReview{curated: curated, config: config}
}

// Spawn fires a non-blocking review. snapshot contains recent turns + existing memory.
func (r *BackgroundReview) Spawn(ctx context.Context, recentTurns []Turn, existingSnapshot string) {
	if !r.config.Enabled {
		return
	}

	go func() {
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

		logging.Infof("BackgroundReview: would review %d turns (memory: %s)",
			len(recentTurns), existingSnapshot[:min(len(existingSnapshot), 50)])
		_ = b.String() // placeholder — real LLM call will be wired in Task 8
	}()
}
