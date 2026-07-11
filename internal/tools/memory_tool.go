package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/liuscraft/orion-x/internal/memory"
)

const MemoryToolName = "memory"

// MemoryToolSpec returns a tool Spec that adds/removes curated memory via the
// per-connection CuratedStore. The agent processes system prompts and tool
// calls through this integrated tool instead of a separate function call.
func MemoryToolSpec(store *memory.CuratedStore) Spec {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []any{"add", "replace", "remove"},
				"description": "操作类型",
			},
			"target": map[string]any{
				"type":        "string",
				"enum":        []any{"memory", "user"},
				"description": "memory = 你的笔记（环境/项目/技巧）；user = 用户画像",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "add 或 replace 时的新内容。简洁、信息密集的句子效果最好。",
			},
			"old_text": map[string]any{
				"type":        "string",
				"description": "replace 或 remove 时的子串匹配。只需要唯一的子串，不必是完整条目。",
			},
		},
		"required": []any{"action", "target"},
	}

	return Spec{
		Name: MemoryToolName,
		Description: "管理长期记忆。你有两个存储空间：\n" +
			"memory（你的笔记：环境事实、项目约定、工具技巧、学到的东西）\n" +
			"user（用户画像：偏好、风格、习惯、期待）\n\n" +
			"何时保存（主动保存，不需要用户要求）：\n" +
			"  用户偏好：「我更喜欢 TypeScript」→ 存到 user\n" +
			"  环境事实：「这台机器装的 Debian 12」→ 存到 memory\n" +
			"  纠正：「不要用 sudo」→ 存到 memory\n" +
			"  约定：「项目用 tab 缩进」→ 存到 memory\n" +
			"  完成的工作：「已将数据库从 MySQL 迁到 PostgreSQL」→ 存到 memory\n" +
			"  明确要求：「记一下我的 API key 每月轮换」→ 存到 memory\n\n" +
			"不要保存：\n" +
			"  显而易见的琐事、易重新发现的事实、原始数据转储、临时会话上下文\n\n" +
			"容量管理：memory 上限 2200 字符，user 上限 1375 字符。\n" +
			"  当使用率超过 80% 时，在追加新条目之前合并或删除。\n" +
			"  合并时用 replace 将相关条目合并为更短的版本。\n\n" +
			"子串匹配：replace/remove 的 old_text 只需要能唯一匹配一条条目的子串。",
		Parameters: params,
		Execute: func(ctx context.Context, args json.RawMessage) (Result, error) {
			var a struct {
				Action  string `json:"action"`
				Target  string `json:"target"`
				Content string `json:"content,omitempty"`
				OldText string `json:"old_text,omitempty"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return Result{}, fmt.Errorf("memory: parse args: %w", err)
			}

			if a.Target != "memory" && a.Target != "user" {
				return Result{Output: mustJSON(map[string]interface{}{
					"success": false, "error": "target 必须是 memory 或 user",
				})}, nil
			}

			var result *memory.ToolResult
			switch a.Action {
			case "add":
				result = store.Add(a.Target, a.Content)
			case "replace":
				result = store.Replace(a.Target, a.OldText, a.Content)
			case "remove":
				result = store.Remove(a.Target, a.OldText)
			default:
				return Result{Output: mustJSON(map[string]interface{}{
					"success": false, "error": "action 必须是 add/replace/remove",
				})}, nil
			}

			return Result{Output: mustJSON(result)}, nil
		},
	}
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
