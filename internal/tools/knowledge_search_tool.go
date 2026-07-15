package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/liuscraft/orion-x/internal/knowledge"
	"github.com/liuscraft/orion-x/internal/logging"
)

// KnowledgeSearchToolName is the tool name for knowledge base search.
const KnowledgeSearchToolName = "knowledge_search"

// KnowledgeSearchToolSpec returns a tool Spec that searches knowledge base documents
// via the Manager's vector search HTTP API. Zero extra LLM cost beyond the embedding call.
func KnowledgeSearchToolSpec(client *knowledge.SearchClient) Spec {
	return Spec{
		Name: KnowledgeSearchToolName,
		Description: "搜索知识库文档。通过语义检索从上传的文档中查找相关内容。\n\n" +
			"knowledge_search 和 memory / session_search 的区别：\n" +
			"  • memory = 你的笔记（偏好、事实、技巧），随时在上下文中可用\n" +
			"  • session_search = 按需回忆历史对话「我们上周讨论过 X 吗？」\n" +
			"  • knowledge_search = 检索用户上传的文档资料，如产品文档、技术手册",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"q": map[string]any{
					"type":        "string",
					"description": "搜索关键词或问题",
				},
				"top_k": map[string]any{
					"type":        "integer",
					"description": "返回结果数（默认 5，最大 10）",
				},
			},
			"required": []any{"q"},
		},
		Execute: func(ctx context.Context, args json.RawMessage) (Result, error) {
			var a struct {
				Q    string `json:"q"`
				TopK int    `json:"top_k,omitempty"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return Result{}, fmt.Errorf("knowledge_search: parse args: %w", err)
			}
			if a.Q == "" {
				return Result{Output: mustJSON(map[string]any{
					"success": false, "error": "q 不能为空",
				})}, nil
			}

			items, err := client.Search(ctx, a.Q, a.TopK)
			if err != nil {
				logging.Warnf("KnowledgeSearch: %v", err)
				return Result{Output: mustJSON(map[string]any{
					"success": false, "error": "检索失败，请稍后重试",
				})}, nil
			}

			return Result{Output: mustJSON(map[string]any{
				"success": true,
				"results": items,
			})}, nil
		},
	}
}
