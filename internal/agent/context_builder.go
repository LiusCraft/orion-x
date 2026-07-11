package agent

import (
	"context"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/session"
)

// buildContextMessages 构建发给 LLM 的完整消息列表：system 消息（优先取 Agent 配
// 置的角色提示词，其次取长期记忆服务提供的上下文，最后退回默认提示词）+ 会话历史。
func (a *Agent) buildContextMessages(ctx context.Context, sess *session.Session) []llm.Message {
	history := sess.ToLLMMessages()

	// Agent 配置的角色提示词优先
	if a.systemPrompt != "" {
		return mergeSystemAndHistory([]llm.Message{{Role: "system", Content: a.systemPrompt}}, history)
	}

	if a.memorySvc != nil {
		memMsgs := a.memorySvc.BuildContextMessages(ctx, nil)
		return mergeSystemAndHistory(filterSystemMessages(memMsgs), history)
	}

	return mergeSystemAndHistory([]llm.Message{{Role: "system", Content: defaultSystemPrompt}}, history)
}

// filterSystemMessages 从记忆服务返回的消息中挑出 system 角色的部分。
func filterSystemMessages(msgs []*llm.Message) []llm.Message {
	result := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			result = append(result, llm.Message{Role: m.Role, Content: m.Content})
		}
	}
	return result
}

// mergeSystemAndHistory 将 system 消息与会话历史拼接为最终发送顺序。
func mergeSystemAndHistory(systemMsgs, history []llm.Message) []llm.Message {
	result := make([]llm.Message, 0, len(systemMsgs)+len(history))
	result = append(result, systemMsgs...)
	return append(result, history...)
}
