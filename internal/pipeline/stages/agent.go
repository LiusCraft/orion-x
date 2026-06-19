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
	agent   AgentRunner
	session *session.Session
}

// NewAgentStage 创建 AgentStage。会话由调用方管理，跨轮次复用。
func NewAgentStage(agent AgentRunner, sess *session.Session) pipeline.Stage {
	return &AgentStage{
		BaseStage: pipeline.NewBaseStage("agent"),
		agent:     agent,
		session:   sess,
	}
}

// Process 处理消息
func (s *AgentStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message)

	go func() {
		defer close(output)

		var pending *pipeline.Message

		for {
			msg, ok := s.nextMessage(ctx, input, pending)
			if !ok {
				return
			}
			pending = nil

			if msg.Type == pipeline.MessageTypeInterrupt {
				select {
				case output <- msg:
				case <-ctx.Done():
					return
				}
				continue
			}

			text, ok := msg.Payload.(string)
			if !ok {
				select {
				case output <- msg:
				case <-ctx.Done():
					return
				}
				continue
			}

			s.session.Add(session.Message{Role: session.RoleUser, Content: text})

			pending = s.runAgentWithInterrupt(ctx, input, output, msg)
		}
	}()

	return output
}

// nextMessage 获取下一条消息：优先返回 pending 消息，否则从 input 读取
func (s *AgentStage) nextMessage(ctx context.Context, input <-chan pipeline.Message, pending *pipeline.Message) (pipeline.Message, bool) {
	if pending != nil {
		return *pending, true
	}
	select {
	case <-ctx.Done():
		return pipeline.Message{}, false
	case msg, ok := <-input:
		return msg, ok
	}
}

// runAgentWithInterrupt 运行 Agent，同时监听 input 中的中断消息。
// 返回在 Agent 运行期间从 input 消费的消息（如有），由主循环处理。
func (s *AgentStage) runAgentWithInterrupt(
	ctx context.Context,
	input <-chan pipeline.Message,
	output chan<- pipeline.Message,
	textMsg pipeline.Message,
) *pipeline.Message {
	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	eventChan, err := s.agent.Run(agentCtx, s.session)
	if err != nil {
		logging.Errorf("AgentStage: agent process error: %v", err)
		select {
		case output <- textMsg.WithError(err):
		case <-ctx.Done():
		}
		return nil
	}

	for {
		select {
		case agentEvent, ok := <-eventChan:
			if !ok {
				return nil
			}
			pipelineMsg := s.convertAgentEvent(agentEvent, textMsg.Metadata)
			select {
			case output <- pipelineMsg:
			case msg, ok := <-input:
				cancel()
				s.drainEventChan(eventChan)
				if !ok {
					return nil
				}
				return &msg
			case <-agentCtx.Done():
				s.drainEventChan(eventChan)
				return nil
			case <-ctx.Done():
				s.drainEventChan(eventChan)
				return nil
			}
		case msg, ok := <-input:
			cancel()
			s.drainEventChan(eventChan)
			if !ok {
				return nil
			}
			return &msg
		case <-ctx.Done():
			cancel()
			s.drainEventChan(eventChan)
			return nil
		}
	}
}

// drainEventChan 排空 agent 事件通道
func (s *AgentStage) drainEventChan(eventChan <-chan agent.AgentEvent) {
	for range eventChan {
	}
}

// convertAgentEvent 转换 AgentEvent 到 Pipeline Message
func (s *AgentStage) convertAgentEvent(event agent.AgentEvent, metadata pipeline.Metadata) pipeline.Message {
	switch e := event.(type) {
	case *agent.TextChunkEvent:
		return pipeline.Message{
			Type:     pipeline.MessageTypeData,
			Payload:  e.Chunk, // string，TTS Stage 通过类型断言消费
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
