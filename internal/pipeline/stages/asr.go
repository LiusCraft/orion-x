package stages

import (
	"context"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/pipeline"
)

// AudioSource is an audio input source that provides raw PCM bytes.
// Defined here because only ASRStage uses it to drive ASRProcessor.Write.
type AudioSource interface {
	Read(ctx context.Context) ([]byte, error)
	Close() error
}

// ASRStage wraps an ASRProcessor as a pipeline source stage.
// If source is non-nil, it reads audio from the source in a background goroutine.
// Otherwise the caller is responsible for pushing audio via proc.Write.
type ASRStage struct {
	*pipeline.BaseStage
	proc   audio.ASRProcessor
	source AudioSource
}

// NewASRStage creates an ASRStage that reads from source.
func NewASRStage(proc audio.ASRProcessor, source AudioSource) pipeline.Stage {
	return &ASRStage{
		BaseStage: pipeline.NewBaseStage("asr"),
		proc:      proc,
		source:    source,
	}
}

func (s *ASRStage) Process(ctx context.Context, input <-chan pipeline.Message) <-chan pipeline.Message {
	output := make(chan pipeline.Message, 16)
	// done 是 ASRStage 内部关闭信号，独立于外部 ctx：forwardInterrupt 必须
	// 在所有会导致 close(output) 的路径上都能被唤醒退出（包括 proc.Start
	// 失败提前返回的分支，此时外部 ctx 未必已经 Done），否则 wg.Wait() 会
	// 死锁。
	done := make(chan struct{})
	var wg sync.WaitGroup

	s.proc.OnResult(func(result audio.ASRResult) {
		if !result.IsFinal {
			return // 中间结果不走 pipeline，外部通过 ASRProcessor 回调直接处理
		}
		select {
		case output <- pipeline.Message{
			Type:    pipeline.MessageTypeData,
			Payload: result.Text,
			Metadata: pipeline.Metadata{
				Timestamp: time.Now(),
			},
		}:
		case <-ctx.Done():
		}
	})

	s.proc.OnSpeechStart(func() {
		select {
		case output <- pipeline.Message{
			Type: pipeline.MessageTypeInterrupt,
			Metadata: pipeline.Metadata{
				Timestamp: time.Now(),
			},
		}:
		case <-ctx.Done():
		}
	})

	// forwardControlMessages 把外部通过 pipeline.Input() 注入的消息转发到
	// output，使其能沿 DAG 传播到下游：
	//   - MessageTypeInterrupt：打断信号（例如 WS 连接收到 abort），下游
	//     AgentStage/TTSStage 已支持响应。
	//   - MessageTypeData：文本直接注入（例如 WS "listen state:detect"
	//     跳过 ASR 直接送一段文本），当作一轮新的用户输入喂给 AgentStage，
	//     同时因为 asr 节点通常也会 fan-out 给一个展示识别文本的下游
	//     （如 ws_output），这段文本也会被当作"用户说的话"回显。
	// Finished/Error 类型不转发，这两种只应该由 pipeline 内部产生。
	// ASRStage 本身在 CLI 场景下从不接收 input（从未调用
	// pipeline.Input()），这个 goroutine 在那种场景下完全惰性。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			case msg, ok := <-input:
				if !ok {
					return
				}
				if msg.Type != pipeline.MessageTypeInterrupt && msg.Type != pipeline.MessageTypeData {
					continue
				}
				select {
				case output <- msg:
				case <-done:
					return
				}
			}
		}
	}()

	go func() {
		defer func() {
			close(done)
			wg.Wait()
			close(output)
		}()

		if err := s.proc.Start(ctx); err != nil {
			logging.Errorf("ASRStage: start error: %v", err)
			select {
			case output <- pipeline.Message{
				Type: pipeline.MessageTypeError,
				Metadata: pipeline.Metadata{
					Error:     err,
					Timestamp: time.Now(),
				},
			}:
			default:
			}
			return
		}

		if s.source != nil {
			go s.readFromSource(ctx)
		}

		<-ctx.Done()
		_ = s.proc.Stop()
	}()

	return output
}

func (s *ASRStage) readFromSource(ctx context.Context) {
	defer func() { _ = s.source.Close() }()
	for {
		data, err := s.source.Read(ctx)
		if err != nil {
			return
		}
		if err := s.proc.Write(data); err != nil {
			return
		}
	}
}
