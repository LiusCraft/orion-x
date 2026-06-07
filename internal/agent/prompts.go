package agent

const defaultSystemPrompt = "你是一个通用 AI 助手。\n\n当用户请求需要外部能力时，请调用合适的工具。\n工具名称必须与提供的工具列表完全一致。"

// DefaultSystemPrompt 暴露给外部组件使用。
func DefaultSystemPrompt() string {
	return defaultSystemPrompt
}
