package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// CreateToolCallGraph 创建工具调用 Agent (线性流程)
// 流程：意图识别(小模型) → 工具调用 → 最终生成(大模型)
func CreateToolCallGraph(ctx context.Context) (compose.Runnable[[]*schema.Message, *schema.Message], error) {
	key := "b69a4b0f05124a59835be2adf1ac84a5.yvFfw6QQNJDXT7IP"
	baseURL := "https://open.bigmodel.cn/api/coding/paas/v4"

	// 1. 小模型：意图识别 + 工具调用
	intentModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   "glm-4-flash",
		APIKey:  key,
	})
	if err != nil {
		return nil, err
	}

	// 2. 大模型：用于多工具调用
	largeModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   "glm-4.7",
		APIKey:  key,
	})
	if err != nil {
		return nil, err
	}

	// 3. 最终生成模型
	finalModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   "glm-4.7",
		APIKey:  key,
	})
	if err != nil {
		return nil, err
	}

	// 4. 创建所有工具
	mockTools, err := CreateMockTools()
	if err != nil {
		return nil, fmt.Errorf("create mock tools failed: %w", err)
	}

	// 转换为 tool.BaseTool 切片
	allTools := make([]tool.BaseTool, 0, len(mockTools))
	for _, t := range mockTools {
		if bt, ok := t.(tool.BaseTool); ok {
			allTools = append(allTools, bt)
		}
	}
	getTimeTool, _ := utils.InferTool("getTime", "获取当前时间", func(_ context.Context, _ struct{}) (string, error) {
		result := time.Now().Format("2006-01-02 15:04:05")
		log.Printf("[Tool] getTime 执行完成，结果: %s", result)
		return result, nil
	})

	getWeatherTool, _ := utils.InferTool("getWeather", "获取指定城市的天气",
		func(ctx context.Context, args struct {
			City string `json:"city"`
		}) (string, error) {
			log.Printf("[ReAct Tool] getWeather 执行，城市: %s", args.City)
			// 模拟天气数据
			return fmt.Sprintf("%s的天气：晴天，温度25°C", args.City), nil
		})

	allTools = append(allTools, getWeatherTool)
	allTools = append(allTools, getTimeTool)

	// 创建工具 ID -> 工具的映射
	toolMap := make(map[string]tool.BaseTool)
	for _, t := range allTools {
		info, _ := t.Info(ctx)
		toolMap[info.Name] = t
	}

	log.Printf("[Init] 已加载 %d 个工具，工具映射大小: %d", len(allTools), len(toolMap))

	// 5. 创建 isMuiltTool 工具（小模型专用，用于识别多工具场景）
	isMuiltTool, _ := utils.InferTool("isMuiltTool", "当用户问题涉及多个工具时，调用此工具。参数 Ids 是需要调用的工具名称列表，用逗号分隔。例如：{\"Ids\": [\"getTime\", \"getWeather\"]}", func(_ context.Context, toolInfos struct {
		Ids []string `json:"Ids"`
	}) (string, error) {
		return fmt.Sprintf("已识别多工具调用: %s", strings.Join(toolInfos.Ids, ",")), nil
	})

	// 小模型绑定所有工具 + isMuiltTool
	smallModelTools := append(allTools, isMuiltTool)
	smallModelToolInfos := make([]*schema.ToolInfo, 0, len(smallModelTools))
	for _, t := range smallModelTools {
		toolInfo, _ := t.Info(ctx)
		smallModelToolInfos = append(smallModelToolInfos, toolInfo)
	}
	intentModel.BindTools(smallModelToolInfos)

	// 6. 创建工具节点（包含所有工具）
	toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: allTools,
	})
	if err != nil {
		return nil, err
	}

	// 7. 用 Lambda 封装整个流程
	agentLambda := compose.InvokableLambda(func(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
		log.Println("=== [Agent] 开始执行 ===")
		log.Printf("[Agent] 输入消息数: %d", len(messages))

		// 1. 小模型意图识别
		log.Println("[IntentModel] 小模型开始分析...")
		intentResp, err := intentModel.Generate(ctx, append(messages, schema.SystemMessage("分析用户问题：如果是单个问题直接调用对应工具；如果是多个问题，必须调用 isMuiltTool 工具，传入需要调用的工具名称列表。不是则直接调用")))
		if err != nil {
			log.Printf("[IntentModel] 调用失败: %v", err)
			return nil, err
		}
		log.Printf("[IntentModel] 响应: %s", intentResp.Content)
		log.Printf("[IntentModel] ToolCalls 数量: %d", len(intentResp.ToolCalls))

		// 2. 如果没有工具调用，直接返回
		if len(intentResp.ToolCalls) == 0 {
			log.Println("[Agent] 无工具调用，直接返回")
			return intentResp, nil
		}

		// 3. 检查是否是多工具调用场景
		var selectedTools []tool.BaseTool
		var finalMessages []*schema.Message

		if intentResp.ToolCalls[0].Function.Name == "isMuiltTool" {
			// 解析工具 ID 列表
			var toolIds struct {
				Ids []string `json:"Ids"`
			}
			if err := json.Unmarshal([]byte(intentResp.ToolCalls[0].Function.Arguments), &toolIds); err != nil {
				log.Printf("[Agent] 解析工具ID失败: %v", err)
				// 降级：从参数字符串中提取
				argStr := intentResp.ToolCalls[0].Function.Arguments
				toolIds.Ids = strings.FieldsFunc(argStr, func(c rune) bool {
					return c == ',' || c == '"' || c == '[' || c == ']' || c == ':'
				})
			}

			log.Printf("[Agent] 检测到多工具调用，工具列表: %v", toolIds.Ids)

			// 根据 ID 找到对应的工具
			for _, toolId := range toolIds.Ids {
				toolId = strings.TrimSpace(toolId)
				if tool, ok := toolMap[toolId]; ok {
					selectedTools = append(selectedTools, tool)
					log.Printf("[Agent]   找到工具: %s", toolId)
				} else {
					log.Printf("[Agent]   未找到工具: %s", toolId)
				}
			}

			if len(selectedTools) == 0 {
				log.Println("[Agent] 没有找到有效工具，使用默认工具")
				selectedTools = allTools[:10] // 降级：使用前10个工具
			}

			// 动态绑定选中的工具到大模型
			selectedToolInfos := make([]*schema.ToolInfo, 0, len(selectedTools))
			for _, t := range selectedTools {
				toolInfo, _ := t.Info(ctx)
				selectedToolInfos = append(selectedToolInfos, toolInfo)
			}
			largeModel.BindTools(selectedToolInfos)
			log.Printf("[Agent] 已绑定 %d 个工具到大模型", len(selectedTools))

			// 构建消息让大模型调用工具
			finalMessages = append([]*schema.Message{}, messages...)
			finalMessages = append(finalMessages, schema.SystemMessage(fmt.Sprintf("用户需要以下操作，请调用对应工具完成: %v", toolIds.Ids)))

			// 大模型生成工具调用
			largeResp, err := largeModel.Generate(ctx, finalMessages)
			if err != nil {
				log.Printf("[LargeModel] 调用失败: %v", err)
				return nil, err
			}

			log.Printf("[LargeModel] ToolCalls 数量: %d", len(largeResp.ToolCalls))

			// 如果大模型没有调用工具，直接返回
			if len(largeResp.ToolCalls) == 0 {
				log.Println("[LargeModel] 无工具调用，直接返回回复")
				return largeResp, nil
			}

			// 用大模型的响应替换小模型的响应
			intentResp = largeResp
		} else {
			// 单工具调用，直接使用小模型的响应
			log.Println("[Agent] 单工具调用场景")
		}

		// 4. 执行工具
		log.Printf("[ToolsNode] 开始执行工具，数量: %d", len(intentResp.ToolCalls))
		for i, tc := range intentResp.ToolCalls {
			log.Printf("[ToolsNode]   [%d] 工具: %s, 参数: %s", i, tc.Function.Name, tc.Function.Arguments)
		}
		toolResults, err := toolsNode.Invoke(ctx, intentResp)
		if err != nil {
			log.Printf("[ToolsNode] 执行失败: %v", err)
			return nil, err
		}
		log.Printf("[ToolsNode] 工具执行完成，结果数: %d", len(toolResults))

		// 5. 合并消息生成最终回复
		finalMessages = make([]*schema.Message, 0, len(messages)+1+len(toolResults))
		finalMessages = append(finalMessages, messages...)
		finalMessages = append(finalMessages, intentResp)
		finalMessages = append(finalMessages, toolResults...)

		// 6. 大模型生成最终回复
		log.Println("[FinalModel] 生成最终回复...")
		finalResp, err := finalModel.Generate(ctx, finalMessages)
		if err != nil {
			log.Printf("[FinalModel] 调用失败: %v", err)
			return nil, err
		}
		log.Printf("[FinalModel] 最终回复: %s", finalResp.Content)
		log.Println("=== [Agent] 执行完成 ===")

		return finalResp, nil
	})

	// 8. 使用 Chain 包装 Lambda
	chain := compose.NewChain[[]*schema.Message, *schema.Message]().
		AppendLambda(agentLambda)

	result, err := chain.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("compile chain failed: %w", err)
	}

	return result, nil
}

// CreateReActAgent 创建 ReAct Agent
// ReAct 循环：思考 → 行动 → 观察 → 思考 ... → 最终答案
func CreateReActAgent(ctx context.Context) (*react.Agent, error) {
	key := "b69a4b0f05124a59835be2adf1ac84a5.yvFfw6QQNJDXT7IP"
	baseURL := "https://open.bigmodel.cn/api/coding/paas/v4"

	// 1. 创建 ChatModel（支持工具调用）
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   "glm-4-flash",
		APIKey:  key,
	})
	if err != nil {
		return nil, err
	}

	// 2. 创建工具
	getTimeTool, _ := utils.InferTool("getTime", "获取当前时间", func(ctx context.Context, _ struct{}) (string, error) {
		result := time.Now().Format("2006-01-02 15:04:05")
		log.Printf("[ReAct Tool] getTime 执行，结果: %s", result)
		return result, nil
	})

	getWeatherTool, _ := utils.InferTool("getWeather", "获取指定城市的天气",
		func(ctx context.Context, args struct {
			City string `json:"city"`
		}) (string, error) {
			log.Printf("[ReAct Tool] getWeather 执行，城市: %s", args.City)
			// 模拟天气数据
			return fmt.Sprintf("%s的天气：晴天，温度25°C", args.City), nil
		})

	toolList := []tool.BaseTool{getTimeTool, getWeatherTool}

	// 3. 获取工具信息并绑定到模型
	useToolInfo := make([]*schema.ToolInfo, 0, len(toolList))
	for _, t := range toolList {
		toolInfo, _ := t.Info(ctx)
		useToolInfo = append(useToolInfo, toolInfo)
	}
	chatModel.BindTools(useToolInfo)

	// 4. 创建 ReAct Agent
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: toolList,
		},
		MaxStep: 10, // 最多循环10次
	})
	if err != nil {
		return nil, fmt.Errorf("create react agent failed: %w", err)
	}

	return agent, nil
}
