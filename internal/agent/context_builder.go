package agent

import (
	"context"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/session"
)

// buildContextMessages 构建发给 LLM 的完整消息列表：soul + rules + 记忆快照 + 会话历史。
// soul/rules 优先取 Manager 下发的配置，空则回退代码内置默认值。
func (a *Agent) buildContextMessages(ctx context.Context, sess *session.Session) []llm.Message {
	history := sess.ToLLMMessages()

	// Resolve soul + rules: config > file > default
	soul := a.soulPrompt
	if soul == "" {
		soul = SoulPrompt()
	}
	rules := a.rulesPrompt
	if rules == "" {
		rules = RulesPrompt()
	}

	if a.memorySvc != nil {
		memMsgs := a.memorySvc.BuildContextMessages(ctx, nil, soul, rules)
		return mergeSystemAndHistory(filterSystemMessages(memMsgs), history)
	}

	// 无记忆服务时，agent 自己拼接
	systemMsgs := make([]llm.Message, 0, 2)
	if soul != "" {
		systemMsgs = append(systemMsgs, llm.Message{
			Role: "system", Content: "═══════════════════ 身份设定 (SOUL) ═══════════════════\n" + soul,
		})
	}
	if rules != "" {
		systemMsgs = append(systemMsgs, llm.Message{Role: "system", Content: rules})
	}
	return mergeSystemAndHistory(systemMsgs, history)
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
