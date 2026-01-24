package main

import (
	"context"
	"log"

	"github.com/liuscraft/orion-x/internal/agent"
)

func main() {
	ctx := context.Background()

	// 创建 VoiceAgent
	voiceAgent, err := agent.NewVoiceAgent(ctx)
	if err != nil {
		log.Fatalf("NewVoiceAgent failed: %v\n", err)
	}

	// 准备测试输入
	input := "北京天气怎么样,用生气的语气告诉我"

	log.Println("=== 开始 VoiceAgent 测试 ===")
	log.Printf("输入: %s\n", input)

	// 处理输入，获取事件流
	eventChan, err := voiceAgent.Process(ctx, input)
	if err != nil {
		log.Fatalf("Process failed: %v\n", err)
	}

	log.Println("=== 流式输出 ===")

	// 处理事件流
	for event := range eventChan {
		switch e := event.(type) {
		case *agent.TextChunkEvent:
			log.Printf("[TextChunk] %s (Emotion: %s)", e.Chunk, e.Emotion)
		case *agent.EmotionChangedEvent:
			log.Printf("[EmotionChanged] %s", e.Emotion)
		case *agent.ToolCallRequestedEvent:
			log.Printf("[ToolCall] Tool: %s, Type: %s, Args: %v", e.Tool, e.ToolType, e.Args)
		case *agent.FinishedEvent:
			if e.Error != nil {
				log.Printf("[Finished] Error: %v", e.Error)
			} else {
				log.Println("[Finished] Successfully")
			}
		}
	}

	log.Println("=== 测试完成 ===")
}
