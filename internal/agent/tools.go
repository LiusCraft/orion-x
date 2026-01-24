package agent

import (
	"fmt"
)

// ToolClassifier 工具分类器
type ToolClassifier struct {
	toolTypes map[string]ToolType
}

func NewToolClassifier() *ToolClassifier {
	return &ToolClassifier{
		toolTypes: map[string]ToolType{
			"getWeather": ToolTypeQuery,
			"getTime":    ToolTypeQuery,
			"search":     ToolTypeQuery,
			"playMusic":  ToolTypeAction,
			"setVolume":  ToolTypeAction,
			"pauseMusic": ToolTypeAction,
		},
	}
}

// GetToolType 获取工具类型
func (c *ToolClassifier) GetToolType(tool string) ToolType {
	if t, ok := c.toolTypes[tool]; ok {
		return t
	}
	return ToolTypeQuery // 默认为查询类
}

// RegisterTool 注册工具
func (c *ToolClassifier) RegisterTool(name string, toolType ToolType) {
	c.toolTypes[name] = toolType
}

// ActionResponseGenerator 动作类工具回复生成器
type ActionResponseGenerator struct {
	responses map[string]func(args map[string]interface{}) string
}

func NewActionResponseGenerator() *ActionResponseGenerator {
	return &ActionResponseGenerator{
		responses: map[string]func(args map[string]interface{}) string{
			"playMusic": func(args map[string]interface{}) string {
				song := args["song"]
				return fmt.Sprintf("正在为您播放%s", song)
			},
			"setVolume": func(args map[string]interface{}) string {
				level := args["level"]
				return fmt.Sprintf("已将音量设置为%s", level)
			},
			"pauseMusic": func(args map[string]interface{}) string {
				return "音乐已暂停"
			},
		},
	}
}

// GenerateResponse 生成动作类工具的回复
func (g *ActionResponseGenerator) GenerateResponse(tool string, args map[string]interface{}) string {
	if gen, ok := g.responses[tool]; ok {
		return gen(args)
	}
	return "好的，正在为您处理"
}

// RegisterGenerator 注册回复生成器
func (g *ActionResponseGenerator) RegisterGenerator(tool string, generator func(args map[string]interface{}) string) {
	g.responses[tool] = generator
}
