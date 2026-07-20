package provider

import "strings"

const (
	DialectGeneric  = "generic"
	DialectDeepSeek = "deepseek"
	DialectQwen     = "qwen"
	DialectMiniMax  = "minimax"
	DialectKimi     = "kimi"
)

// InferDialect resolves the OpenAI-compatible request dialect from the model
// identifier. Model IDs are the sole source of truth; adapters only select the
// wire protocol used to call the model.
func InferDialect(modelID string) string {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	switch {
	case strings.Contains(modelID, "deepseek"):
		return DialectDeepSeek
	case strings.Contains(modelID, "qwen"):
		return DialectQwen
	case strings.Contains(modelID, "minimax"):
		return DialectMiniMax
	case strings.Contains(modelID, "kimi"), strings.Contains(modelID, "moonshot"):
		return DialectKimi
	default:
		return DialectGeneric
	}
}
