package pipeline

import (
	"context"
	"errors"
	"sync"
)

var ErrNotStarted = errors.New("pipeline is not started")

// Pipeline 流式数据处理管道
type Pipeline interface {
	// Start 启动 Pipeline
	Start(ctx context.Context) error

	// Stop 停止 Pipeline（优雅关闭）
	Stop() error

	// Interrupt 立即打断当前处理
	Interrupt() error

	// Output 获取 Pipeline 输出流
	Output() <-chan Message

	// Input 获取 Pipeline 输入流（用于发送消息）
	Input() chan<- Message

	// Emit injects a message into the final output stream without sending it
	// through source stages.
	Emit(Message) error
}

// PipelineObserver 观察 Pipeline 事件
type PipelineObserver interface {
	OnMessage(stageName string, msg Message)
	OnError(stageName string, err error)
	OnStageStart(stageName string)
	OnStageStop(stageName string)
}

// linearPipeline 线性 Pipeline 实现
type linearPipeline struct {
	stages      []Stage
	input       chan Message
	output      chan Message
	emitted     chan Message
	ctx         context.Context
	cancel      context.CancelFunc
	observer    PipelineObserver
	wg          sync.WaitGroup
	mu          sync.Mutex
	started     bool
	inputClosed bool
}

// Start 启动 Pipeline
func (p *linearPipeline) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = true
	p.mu.Unlock()

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.emitted = make(chan Message, 16)

	// 连接各 Stage
	var currentInput <-chan Message = p.input

	for _, stage := range p.stages {
		if p.observer != nil {
			p.observer.OnStageStart(stage.Name())
		}

		stageOutput := stage.Process(p.ctx, currentInput)

		// 插入观察者（如果需要）
		if p.observer != nil {
			stageOutput = p.wrapWithObserver(stage.Name(), stageOutput)
		}

		currentInput = stageOutput
	}

	// 最后一个 Stage 的输出作为 Pipeline 输出
	p.output = make(chan Message)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer close(p.output)

		for {
			select {
			case <-p.ctx.Done():
				return
			case msg := <-p.emitted:
				select {
				case p.output <- msg:
				case <-p.ctx.Done():
					return
				}
			case msg, ok := <-currentInput:
				if !ok {
					return
				}
				select {
				case p.output <- msg:
				case <-p.ctx.Done():
					return
				}
			}
		}
	}()

	return nil
}

// wrapWithObserver 包装 Stage 输出，添加观察者
func (p *linearPipeline) wrapWithObserver(stageName string, input <-chan Message) <-chan Message {
	output := make(chan Message)

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer close(output)

		for {
			select {
			case <-p.ctx.Done():
				return
			case msg, ok := <-input:
				if !ok {
					return
				}

				// 通知观察者
				p.observer.OnMessage(stageName, msg)
				if msg.IsError() {
					p.observer.OnError(stageName, msg.Metadata.Error)
				}

				// 传递消息
				select {
				case output <- msg:
				case <-p.ctx.Done():
					return
				}
			}
		}
	}()

	return output
}

// Stop 停止 Pipeline
func (p *linearPipeline) Stop() error {
	p.mu.Lock()

	if !p.started {
		p.mu.Unlock()
		return nil
	}

	if p.cancel != nil {
		p.cancel()
	}

	// 关闭输入 channel（如果还未关闭）
	if !p.inputClosed {
		close(p.input)
		close(p.emitted)
		p.inputClosed = true
	}

	p.mu.Unlock()

	// 等待所有 goroutine 退出
	p.wg.Wait()

	p.mu.Lock()
	// 通知观察者
	if p.observer != nil {
		for _, stage := range p.stages {
			p.observer.OnStageStop(stage.Name())
		}
	}

	p.started = false
	p.mu.Unlock()
	return nil
}

// Interrupt 立即打断
func (p *linearPipeline) Interrupt() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel != nil {
		p.cancel()
	}

	return nil
}

// Output 获取输出流
func (p *linearPipeline) Output() <-chan Message {
	return p.output
}

// Input 获取输入流
func (p *linearPipeline) Input() chan<- Message {
	return p.input
}

func (p *linearPipeline) Emit(msg Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.started || p.ctx == nil {
		return ErrNotStarted
	}
	select {
	case p.emitted <- msg:
		return nil
	case <-p.ctx.Done():
		return ErrNotStarted
	}
}
