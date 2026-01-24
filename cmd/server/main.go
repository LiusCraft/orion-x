package main

import (
	"context"
	"log"

	"github.com/cloudwego/eino/schema"
	"github.com/liuscraft/orion-x/internal/ai"
)

func main() {
	ctx := context.Background()

	// 创建 ReAct Agent
	agent, err := ai.CreateToolCallGraph(ctx)
	if err != nil {
		log.Printf("Create ReAct Agent failed: %v\n", err)
		return
	}

	// 准备测试消息
	messages := []*schema.Message{
		schema.UserMessage("现在几点？生成个代码，天气怎么样，提醒我明天上班"),
	}

	log.Println("=== 开始 ReAct Agent 测试 ===")

	// 使用 Generate 方法（同步）
	result, err := agent.Invoke(ctx, messages)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	log.Println("=== 最终回复 ===")
	log.Println(result.Content)

	// sreader, err := llm.Stream(ctx, messages)
	// if err != nil {
	// 	log.Printf("Error: %v\n", err)
	// 	return
	// }
	// for {
	// 	message, err := sreader.Recv()
	// 	if err == io.EOF {
	// 		break
	// 	}
	// 	if err != nil {
	// 		log.Printf("Error: %v\n", err)
	// 		return
	// 	}
	// 	log.Printf("Message: %+v\n", message)
	// }
}
