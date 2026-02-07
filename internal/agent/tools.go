package agent

import (
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/internal/tools"
)

// ActionResponseGenerator 动作类工具回复生成器
type ActionResponseGenerator struct {
	responses map[string]func(args map[string]interface{}) string
}

func NewActionResponseGenerator() *ActionResponseGenerator {
	return NewActionResponseGeneratorWithTemplates(nil)
}

func NewActionResponseGeneratorWithTemplates(templates map[string]string) *ActionResponseGenerator {
	generator := &ActionResponseGenerator{
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
	for tool, template := range templates {
		tmpl := template
		generator.responses[tool] = func(args map[string]interface{}) string {
			return applyTemplate(tmpl, args)
		}
	}
	return generator
}

// NewActionResponseGeneratorWithToolManager 使用 ToolManager 创建回复生成器
func NewActionResponseGeneratorWithToolManager(tm tools.ToolManager) *ActionResponseGenerator {
	generator := &ActionResponseGenerator{
		responses: make(map[string]func(args map[string]interface{}) string),
	}

	// 从 ToolManager 获取所有工具的动作响应模板
	for _, name := range getToolNames(tm) {
		if template, ok := tm.GetActionResponse(name); ok {
			tmpl := template
			generator.responses[name] = func(args map[string]interface{}) string {
				return applyTemplate(tmpl, args)
			}
		}
	}

	return generator
}

func getToolNames(tm tools.ToolManager) []string {
	infos := tm.ToolInfos()
	names := make([]string, len(infos))
	for i, info := range infos {
		names[i] = info.Name
	}
	return names
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

func ParseToolType(value string) (ToolType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "query":
		return ToolTypeQuery, nil
	case "action":
		return ToolTypeAction, nil
	default:
		return ToolTypeQuery, fmt.Errorf("unknown tool type: %s", value)
	}
}

func ParseToolTypes(values map[string]string) (map[string]ToolType, error) {
	parsed := make(map[string]ToolType, len(values))
	for name, value := range values {
		toolType, err := ParseToolType(value)
		if err != nil {
			return nil, fmt.Errorf("invalid tool type for %s: %w", name, err)
		}
		parsed[name] = toolType
	}
	return parsed, nil
}

func applyTemplate(template string, args map[string]interface{}) string {
	result := template
	for key, value := range args {
		placeholder := "{{" + key + "}}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprint(value))
	}
	return result
}
