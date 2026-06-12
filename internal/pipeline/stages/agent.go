package stages

import (
	"context"
	"fmt"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/pipeline"
	"github.com/liuscraft/orion-x/internal/session"
)

// AgentRunner Agent 运行接口
type AgentRunner interface {
	Run(ctx context.Context, sess *session.Session) (<-chan agent.AgentEvent, error)
}

// AgentStage Agent 处理 Stage
type AgentStage struct {
	*pipeline.BaseStage
	agent AgentRunner
}

// NewAgentStage 创建 AgentStage
func NewAgentStage(agent AgentRunner) pipeline.Stage {
	return &AgentStage{
		BaseStage: pipeline.NewBaseStage("agent"),
		agent:     agent,
	}
}

// Process 处理消息
func (s *AgentStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message)

	go func() {
		defer close(output)

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-input:
				if !ok {
					return
				}

				// 只处理文本输入（用户输入）
				if msg.Type != pipeline.MessageTypeTextChunk {
					select {
					case output <- msg:
					case <-ctx.Done():
						return
					}
					continue
				}

				// 提取文本
				text, ok := msg.Payload.(string)
				if !ok {
					logging.Errorf("AgentStage: invalid payload type, expected string")
					select {
					case output <- msg.WithError(fmt.Errorf("invalid payload type")):
					case <-ctx.Done():
						return
					}
					continue
				}

				// 创建 session 并添加用户消息
				sess := session.New(session.SessionMeta{Model: "default"})
				sess.Add(session.Message{Role: session.RoleUser, Content: text})

				// 调用 Agent
				eventChan, err := s.agent.Run(ctx, sess)
				if err != nil {
					logging.Errorf("AgentStage: agent process error: %v", err)
					select {
					case output <- msg.WithError(err):
					case <-ctx.Done():
						return
					}
					continue
				}

				// 转换 AgentEvent -> Pipeline Message
				for agentEvent := range eventChan {
					pipelineMsg := s.convertAgentEvent(agentEvent, msg.Metadata)
					select {
					case output <- pipelineMsg:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return output
}

// convertAgentEvent 转换 AgentEvent 到 Pipeline Message
func (s *AgentStage) convertAgentEvent(event agent.AgentEvent, metadata pipeline.Metadata) pipeline.Message {
	switch e := event.(type) {
	case *agent.TextChunkEvent:
		return pipeline.Message{
			Type:     pipeline.MessageTypeTextChunk,
			Payload:  e.Chunk,
			Metadata: metadata,
		}

	case *agent.FinishedEvent:
		msg := pipeline.Message{
			Type:     pipeline.MessageTypeFinished,
			Metadata: metadata,
		}
		if e.Error != nil {
			msg.Metadata.Error = e.Error
		}
		return msg

	default:
		logging.Warnf("AgentStage: unknown agent event type: %T", event)
		return pipeline.Message{
			Type:     pipeline.MessageTypeError,
			Metadata: metadata.WithError(fmt.Errorf("unknown agent event type: %T", event)),
		}
	}
}
