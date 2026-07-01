package agent

import (
	"context"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/session"
)

// buildContextMessages 构建发给 LLM 的完整消息列表：system 消息（优先取长期记忆
// 服务提供的上下文，失败或未启用时退回默认提示词）+ 会话历史。
func (a *Agent) buildContextMessages(ctx context.Context, sess *session.Session) []llm.Message {
	history := sess.ToLLMMessages()

	if a.memorySvc != nil {
		memMsgs, err := a.memorySvc.BuildContextMessages(ctx, "")
		if err != nil {
			logging.Warnf("Agent: build memory context failed: %v", err)
		} else {
			return mergeSystemAndHistory(filterSystemMessages(memMsgs), history)
		}
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
