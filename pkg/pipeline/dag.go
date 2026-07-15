package pipeline

import (
	"context"
	"fmt"
	"sync"
)

// dagPipeline DAG 拓扑 Pipeline 实现
// 支持 fan-out（一对多）和 fan-in（多对一）
type dagPipeline struct {
	nodes map[string]Stage
	edges map[string][]string // from -> downstream nodes

	input  chan Message
	output chan Message

	ctx    context.Context
	cancel context.CancelFunc

	observer      PipelineObserver
	wg            sync.WaitGroup
	mu            sync.Mutex
	started       bool
	inputClosed   bool
	inputBufSize  int
	fanoutBufSize int
}

// newDAGPipeline 创建 DAG Pipeline
func newDAGPipeline(
	nodes map[string]Stage,
	edges map[string][]string,
	observer PipelineObserver,
	inputBufSize, fanoutBufSize int,
) *dagPipeline {
	return &dagPipeline{
		nodes:         nodes,
		edges:         edges,
		observer:      observer,
		inputBufSize:  inputBufSize,
		fanoutBufSize: fanoutBufSize,
	}
}

// Start 启动 DAG Pipeline
func (d *dagPipeline) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return nil
	}
	d.started = true
	d.mu.Unlock()

	// 验证 DAG 并获取拓扑序
	order, err := d.topologicalSort()
	if err != nil {
		d.mu.Lock()
		d.started = false
		d.mu.Unlock()
		return fmt.Errorf("invalid DAG: %w", err)
	}

	d.ctx, d.cancel = context.WithCancel(ctx)
	d.input = make(chan Message, d.inputBufSize)

	// nodeOutputs: 每个节点处理后的输出 channel
	nodeOutputs := make(map[string]<-chan Message)

	// nodeInputSources: 每个节点的上游输入 channel 列表（用于 fan-in 合并）
	nodeInputSources := make(map[string][]<-chan Message)
	for name := range d.nodes {
		nodeInputSources[name] = nil
	}

	// Source 节点从 pipeline Input 读取
	for name := range d.nodes {
		if !d.hasIncomingEdges(name) {
			nodeInputSources[name] = append(nodeInputSources[name], d.input)
		}
	}

	// 按拓扑序依次启动节点
	for _, name := range order {
		node := d.nodes[name]

		// 构建该节点的输入
		var mergedInput <-chan Message
		sources := nodeInputSources[name]
		switch len(sources) {
		case 0:
			// 无输入源（不应发生在 valid DAG 中，但做防御）
			ch := make(chan Message)
			close(ch)
			mergedInput = ch
		case 1:
			mergedInput = sources[0]
		default:
			mergedInput = d.mergeChannels(sources)
		}

		// 插入观察者
		if d.observer != nil {
			d.observer.OnStageStart(name)
			mergedInput = d.wrapWithObserver(name, mergedInput)
		}

		// 运行 Stage
		stageOutput := node.Process(d.ctx, mergedInput)
		nodeOutputs[name] = stageOutput

		// Fan-out: 将输出连接到下游节点
		downstreams := d.edges[name]
		switch len(downstreams) {
		case 0:
			// Sink 节点：输出仅归入 pipeline Output，无需 fan-out
		case 1:
			// 单下游：直接传递
			nodeInputSources[downstreams[0]] = append(
				nodeInputSources[downstreams[0]], stageOutput)
		default:
			// 多下游：单 reader 写多个 writer，确保每消息复制到所有分支
			fanOutChs := make([]chan Message, len(downstreams))
			for i, ds := range downstreams {
				fanOutChs[i] = make(chan Message, d.fanoutBufSize)
				nodeInputSources[ds] = append(nodeInputSources[ds], fanOutChs[i])
			}
			d.wg.Add(1)
			go d.broadcastMessages(stageOutput, fanOutChs)
		}
	}

	// 收集所有 sink 节点输出，合并为 pipeline Output
	var sinkOutputs []<-chan Message
	for name := range d.nodes {
		if !d.hasOutgoingEdges(name) {
			if out, ok := nodeOutputs[name]; ok {
				sinkOutputs = append(sinkOutputs, out)
			}
		}
	}

	d.output = make(chan Message)
	d.wg.Add(1)
	go d.drainOutput(sinkOutputs)

	return nil
}

// broadcastMessages 将源 channel 的每条消息广播到所有目标 channel
func (d *dagPipeline) broadcastMessages(src <-chan Message, dsts []chan Message) {
	defer d.wg.Done()
	for _, dst := range dsts {
		defer close(dst)
	}

	for {
		select {
		case <-d.ctx.Done():
			return
		case msg, ok := <-src:
			if !ok {
				return
			}
			for _, dst := range dsts {
				select {
				case dst <- msg:
				case <-d.ctx.Done():
					return
				}
			}
		}
	}
}

// mergeChannels 合并多个 channel 为一个
func (d *dagPipeline) mergeChannels(channels []<-chan Message) <-chan Message {
	out := make(chan Message)

	var wg sync.WaitGroup
	wg.Add(len(channels))
	for _, ch := range channels {
		go func(c <-chan Message) {
			defer wg.Done()
			for {
				select {
				case <-d.ctx.Done():
					return
				case msg, ok := <-c:
					if !ok {
						return
					}
					select {
					case out <- msg:
					case <-d.ctx.Done():
						return
					}
				}
			}
		}(ch)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// drainOutput 合并所有 sink 输出到 pipeline Output
func (d *dagPipeline) drainOutput(sinkOutputs []<-chan Message) {
	defer d.wg.Done()
	defer close(d.output)

	if len(sinkOutputs) == 0 {
		return
	}

	var merged <-chan Message
	if len(sinkOutputs) == 1 {
		merged = sinkOutputs[0]
	} else {
		merged = d.mergeChannels(sinkOutputs)
	}

	for {
		select {
		case <-d.ctx.Done():
			return
		case msg, ok := <-merged:
			if !ok {
				return
			}
			select {
			case d.output <- msg:
			case <-d.ctx.Done():
				return
			}
		}
	}
}

// wrapWithObserver 包装输入 channel，添加观察者
func (d *dagPipeline) wrapWithObserver(stageName string, input <-chan Message) <-chan Message {
	output := make(chan Message)

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer close(output)

		for {
			select {
			case <-d.ctx.Done():
				return
			case msg, ok := <-input:
				if !ok {
					return
				}

				d.observer.OnMessage(stageName, msg)
				if msg.IsError() {
					d.observer.OnError(stageName, msg.Metadata.Error)
				}

				select {
				case output <- msg:
				case <-d.ctx.Done():
					return
				}
			}
		}
	}()

	return output
}

// hasIncomingEdges 判断节点是否有入边
func (d *dagPipeline) hasIncomingEdges(name string) bool {
	for _, tos := range d.edges {
		for _, to := range tos {
			if to == name {
				return true
			}
		}
	}
	return false
}

// hasOutgoingEdges 判断节点是否有出边
func (d *dagPipeline) hasOutgoingEdges(name string) bool {
	tos, ok := d.edges[name]
	return ok && len(tos) > 0
}

// Stop 停止 Pipeline
func (d *dagPipeline) Stop() error {
	d.mu.Lock()
	if !d.started {
		d.mu.Unlock()
		return nil
	}

	if d.cancel != nil {
		d.cancel()
	}

	if !d.inputClosed {
		close(d.input)
		d.inputClosed = true
	}

	d.mu.Unlock()

	d.wg.Wait()

	d.mu.Lock()
	if d.observer != nil {
		for name := range d.nodes {
			d.observer.OnStageStop(name)
		}
	}
	d.started = false
	d.mu.Unlock()

	return nil
}

// Interrupt 立即打断
func (d *dagPipeline) Interrupt() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cancel != nil {
		d.cancel()
	}

	return nil
}

// Output 获取输出流
func (d *dagPipeline) Output() <-chan Message {
	return d.output
}

// Input 获取输入流
func (d *dagPipeline) Input() chan<- Message {
	return d.input
}

// topologicalSort Kahn 算法拓扑排序，检测环
func (d *dagPipeline) topologicalSort() ([]string, error) {
	inDegree := make(map[string]int)
	for name := range d.nodes {
		inDegree[name] = 0
	}
	for _, tos := range d.edges {
		for _, to := range tos {
			if _, ok := d.nodes[to]; !ok {
				return nil, fmt.Errorf("unknown node: %s", to)
			}
			inDegree[to]++
		}
	}

	queue := make([]string, 0)
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		for _, next := range d.edges[node] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(order) != len(d.nodes) {
		return nil, fmt.Errorf("cycle detected in pipeline DAG")
	}

	return order, nil
}
