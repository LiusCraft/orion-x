package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/liuscraft/orion-x/internal/logging"
)

const SessionSearchToolName = "session_search"

// SessionSearchToolSpec returns a tool Spec that searches historical session
// records via the Manager's FTS HTTP API. Zero LLM cost.
func SessionSearchToolSpec(managerURL, deviceID string) Spec {
	client := &http.Client{}

	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"q": map[string]any{
				"type":        "string",
				"description": "发现模式：搜索关键词。FTS 全文检索，支持短语匹配",
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "查看模式：要查看的会话 ID",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "最大返回结果数（默认 3，最大 10）",
			},
		},
	}

	return Spec{
		Name: SessionSearchToolName,
		Description: "搜索历史会话记录。使用全文检索，零 LLM 成本。\n\n" +
			"根据传参自动推断模式：\n" +
			"  • 传 q= → 发现模式（搜索匹配的会话）\n" +
			"  • 传 session_id → 查看模式（获取指定会话详情）\n" +
			"  • 什么都不传 → 浏览模式（最近会话列表）\n\n" +
			"session_search 和 memory 的区别：\n" +
			"  • memory = 关键事实，随时在上下文中可用\n" +
			"  • session_search = 按需回忆「我们上周讨论过 X 吗？」",
		Parameters: params,
		Execute: func(ctx context.Context, args json.RawMessage) (Result, error) {
			var a struct {
				Q         string `json:"q,omitempty"`
				SessionID string `json:"session_id,omitempty"`
				Limit     int    `json:"limit,omitempty"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return Result{}, fmt.Errorf("session_search: parse args: %w", err)
			}

			baseURL := strings.TrimRight(managerURL, "/")
			base := fmt.Sprintf("%s/internal/devices/%s", baseURL, deviceID)

			var url string
			if a.SessionID != "" {
				url = fmt.Sprintf("%s/sessions/%s", base, a.SessionID)
			} else if a.Q != "" {
				limit := a.Limit
				if limit <= 0 || limit > 10 {
					limit = 3
				}
				url = fmt.Sprintf("%s/turns?q=%s&limit=%d", base, a.Q, limit)
			} else {
				url = fmt.Sprintf("%s/turns", base)
			}

			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return Result{Output: mustJSON(map[string]interface{}{
					"success": false, "error": "session_search: create request failed",
				})}, nil
			}

			resp, err := client.Do(req)
			if err != nil {
				logging.Warnf("SessionSearch: HTTP error: %v", err)
				return Result{Output: mustJSON(map[string]interface{}{
					"success": false, "error": "检索失败，请稍后重试",
				})}, nil
			}
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return Result{Output: mustJSON(map[string]interface{}{
					"success": false, "error": "读取响应失败",
				})}, nil
			}

			return Result{Output: string(body)}, nil
		},
	}
}
