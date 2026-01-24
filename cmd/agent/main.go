package main

import (
	"context"
	"fmt"
	"os"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/logging"
)

func main() {
	if err := logging.InitFromEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logging.Sync()
	logging.SetTraceID(logging.NewTraceID())

	ctx := context.Background()

	// 创建 VoiceAgent
	voiceAgent, err := agent.NewVoiceAgent(ctx)
	if err != nil {
		logging.Fatalf("NewVoiceAgent failed: %v", err)
	}

	// 准备测试输入
	input := "北京天气怎么样,用生气的语气告诉我"

	logging.Infof("=== 开始 VoiceAgent 测试 ===")
	logging.Infof("输入: %s", input)

	// 处理输入，获取事件流
	eventChan, err := voiceAgent.Process(ctx, input)
	if err != nil {
		logging.Fatalf("Process failed: %v", err)
	}

	logging.Infof("=== 流式输出 ===")

	// 处理事件流
	for event := range eventChan {
		switch e := event.(type) {
		case *agent.TextChunkEvent:
			logging.Infof("[TextChunk] %s (Emotion: %s)", e.Chunk, e.Emotion)
		case *agent.EmotionChangedEvent:
			logging.Infof("[EmotionChanged] %s", e.Emotion)
		case *agent.ToolCallRequestedEvent:
			logging.Infof("[ToolCall] Tool: %s, Type: %s, Args: %v", e.Tool, e.ToolType, e.Args)
		case *agent.FinishedEvent:
			if e.Error != nil {
				logging.Errorf("[Finished] Error: %v", e.Error)
			} else {
				logging.Infof("[Finished] Successfully")
			}
		}
	}

	logging.Infof("=== 测试完成 ===")
}
