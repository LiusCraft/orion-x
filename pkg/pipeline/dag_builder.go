package pipeline

import "fmt"

// DAGBuilder DAG Pipeline 构建器
type DAGBuilder interface {
	AddStage(stage Stage) DAGBuilder
	Connect(from, to string) DAGBuilder
	SetObserver(observer PipelineObserver) DAGBuilder
	SetInputBufferSize(size int) DAGBuilder
	SetFanoutBufferSize(size int) DAGBuilder
	Build() (Pipeline, error)
}

type dagBuilder struct {
	nodes         map[string]Stage
	edges         map[string][]string
	observer      PipelineObserver
	inputBufSize  int
	fanoutBufSize int
}

// NewDAGBuilder 创建 DAG Pipeline 构建器
func NewDAGBuilder() DAGBuilder {
	return &dagBuilder{
		nodes:         make(map[string]Stage),
		edges:         make(map[string][]string),
		fanoutBufSize: 16, // 默认 fan-out 缓冲
	}
}

// AddStage 添加 Stage 节点，以 Stage.Name() 作为节点标识
func (b *dagBuilder) AddStage(stage Stage) DAGBuilder {
	b.nodes[stage.Name()] = stage
	return b
}

// Connect 连接边：from → to
func (b *dagBuilder) Connect(from, to string) DAGBuilder {
	b.edges[from] = append(b.edges[from], to)
	return b
}

// SetObserver 设置观察者
func (b *dagBuilder) SetObserver(observer PipelineObserver) DAGBuilder {
	b.observer = observer
	return b
}

// SetInputBufferSize 设置输入缓冲区大小
func (b *dagBuilder) SetInputBufferSize(size int) DAGBuilder {
	b.inputBufSize = size
	return b
}

// SetFanoutBufferSize 设置 fan-out 分支缓冲区大小
func (b *dagBuilder) SetFanoutBufferSize(size int) DAGBuilder {
	b.fanoutBufSize = size
	return b
}

// Build 构建 DAG Pipeline，验证 DAG 合法性
func (b *dagBuilder) Build() (Pipeline, error) {
	if len(b.nodes) == 0 {
		return nil, fmt.Errorf("DAG must have at least one stage")
	}

	// 验证所有边引用的节点存在
	for from, tos := range b.edges {
		if _, ok := b.nodes[from]; !ok {
			return nil, fmt.Errorf("unknown source node: %s", from)
		}
		for _, to := range tos {
			if _, ok := b.nodes[to]; !ok {
				return nil, fmt.Errorf("unknown target node: %s", to)
			}
		}
	}

	// 复制 nodes 和 edges（避免 builder 后续修改影响已构建的 pipeline）
	nodesCopy := make(map[string]Stage, len(b.nodes))
	for name, stage := range b.nodes {
		nodesCopy[name] = stage
	}

	edgesCopy := make(map[string][]string, len(b.edges))
	for from, tos := range b.edges {
		cp := make([]string, len(tos))
		copy(cp, tos)
		edgesCopy[from] = cp
	}

	p := newDAGPipeline(nodesCopy, edgesCopy, b.observer, b.inputBufSize, b.fanoutBufSize)

	// 预验证拓扑排序
	if _, err := p.topologicalSort(); err != nil {
		return nil, err
	}

	return p, nil
}
