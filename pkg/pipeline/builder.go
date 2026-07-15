package pipeline

// Builder Pipeline 构建器
type Builder interface {
	AddStage(stage Stage) Builder
	SetObserver(observer PipelineObserver) Builder
	SetInputBufferSize(size int) Builder
	Build() Pipeline
}

type builder struct {
	stages          []Stage
	observer        PipelineObserver
	inputBufferSize int
}

// NewBuilder 创建 Pipeline 构建器
func NewBuilder() Builder {
	return &builder{
		stages:          []Stage{},
		inputBufferSize: 0, // 默认无缓冲
	}
}

// AddStage 添加 Stage
func (b *builder) AddStage(stage Stage) Builder {
	b.stages = append(b.stages, stage)
	return b
}

// SetObserver 设置观察者
func (b *builder) SetObserver(observer PipelineObserver) Builder {
	b.observer = observer
	return b
}

// SetInputBufferSize 设置输入缓冲区大小
func (b *builder) SetInputBufferSize(size int) Builder {
	b.inputBufferSize = size
	return b
}

// Build 构建 Pipeline
func (b *builder) Build() Pipeline {
	return &linearPipeline{
		stages:   b.stages,
		input:    make(chan Message, b.inputBufferSize),
		observer: b.observer,
	}
}
