package agent

import (
	"errors"
	"strings"
)

var (
	errLLMAPIKeyRequired  = errors.New("llm api_key is required")
	errLLMBaseURLRequired = errors.New("llm base_url is required")
	errLLMModelRequired   = errors.New("llm model is required")
)

type Config struct {
	Provider    string
	APIKey      string
	BaseURL     string
	Model       string
	SoulPrompt  string // 身份设定，来自 Manager 下发；空 = 代码内置默认值
	RulesPrompt string // 行为规则，来自 Manager 下发；空 = 代码内置默认值
	ExtraFields map[string]any
}

func normalizeConfig(cfg Config) (Config, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return Config{}, errLLMAPIKeyRequired
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		cfg.Provider = "openai"
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return Config{}, errLLMBaseURLRequired
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return Config{}, errLLMModelRequired
	}
	return cfg, nil
}
