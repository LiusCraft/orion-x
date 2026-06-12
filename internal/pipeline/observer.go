package pipeline

import "github.com/liuscraft/orion-x/internal/logging"

// loggingObserver 日志观察者实现
type loggingObserver struct {
	verbose bool
}

// NewLoggingObserver 创建日志观察者
func NewLoggingObserver(verbose bool) PipelineObserver {
	return &loggingObserver{verbose: verbose}
}

func (o *loggingObserver) OnMessage(stageName string, msg Message) {
	if !o.verbose {
		return
	}
	if msg.IsError() {
		return // 错误单独在 OnError 中记录
	}
	logging.Infof("Pipeline [%s]: message type=%s", stageName, msg.Type)
}

func (o *loggingObserver) OnError(stageName string, err error) {
	logging.Errorf("Pipeline [%s]: error: %v", stageName, err)
}

func (o *loggingObserver) OnStageStart(stageName string) {
	logging.Infof("Pipeline: stage [%s] started", stageName)
}

func (o *loggingObserver) OnStageStop(stageName string) {
	logging.Infof("Pipeline: stage [%s] stopped", stageName)
}

// noopObserver 空观察者（用于测试）
type noopObserver struct{}

func (o *noopObserver) OnMessage(stageName string, msg Message)     {}
func (o *noopObserver) OnError(stageName string, err error)          {}
func (o *noopObserver) OnStageStart(stageName string)                {}
func (o *noopObserver) OnStageStop(stageName string)                 {}

// NewNoopObserver 创建空观察者
func NewNoopObserver() PipelineObserver {
	return &noopObserver{}
}
