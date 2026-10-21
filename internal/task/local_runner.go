package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/tools"
)

// defaultMaxSteps 与对话 Agent 的默认步数上限保持一致。
const defaultMaxSteps = 10

// taskSystemPrompt 是任务 loop 的系统提示。任务代理没有对话伙伴，
// 必须自主决策；需要上下文时依赖可用工具自行获取。
const taskSystemPrompt = `你是后台任务执行器，独立完成用户交给你的任务。用户不在对话中，无法回答你的问题。
规则：
1. 不要反问或请求确认；缺少信息时使用可用工具自行获取，取不到就基于已知信息给出最佳结果。
2. 需要多步操作时逐步调用工具，并在每步之后确认结果是否符合预期。
3. 完成后用简洁的自然语言总结最终结果，不要暴露内部工具细节，除非与结果直接相关。`

// LocalConfig 是进程内 loop 的构造参数。
type LocalConfig struct {
	// Client 是任务 loop 使用的 LLM 客户端，必填。
	Client llm.Client
	// Tools 在每次 Start 时快照工具集，让任务代理看到当时已注册的工具。
	// 快照实现可以返回共享 registry 或克隆（渠道层通常传 CloneRegistry）。
	Tools func() *tools.Registry
	// Model 仅用于日志与排查。
	Model string
	// MaxSteps 是 LLM 调用步数上限，<=0 时使用 defaultMaxSteps。
	MaxSteps int
	// Thinking / Options / MaxOutputTokens 透传给每次 LLM 请求，
	// 与主 Agent 使用同一套渠道配置。
	Thinking        llm.ThinkingConfig
	Options         []byte
	MaxOutputTokens int
}

// LocalRunner 是 MVP 的进程内执行后端：一个简单的「LLM + 工具」循环。
// 它有意不依赖对话 Agent 的记忆与 pipeline 能力，后续远程后端接入时
// 保持同一 Runner 接口。
type LocalRunner struct {
	ctx context.Context
	cfg LocalConfig
}

// NewLocalRunner 创建进程内执行后端。ctx 是执行生命周期的根上下文，
// 通常传渠道的 root ctx；单个任务的取消由 Run.Close 派生。
func NewLocalRunner(ctx context.Context, cfg LocalConfig) *LocalRunner {
	return &LocalRunner{ctx: ctx, cfg: cfg}
}

// Start 实现 Runner。
func (r *LocalRunner) Start(req RunRequest) (Run, error) {
	if r == nil || r.cfg.Client == nil {
		return nil, errors.New("local runner requires an llm client")
	}
	if r.cfg.Tools == nil {
		return nil, errors.New("local runner requires a tool registry supplier")
	}
	ctx, cancel := context.WithCancel(r.ctx)
	logging.Infof("task %s: local runner started (model=%q, max_steps=%d)", req.TaskID, r.cfg.Model, r.maxSteps())
	run := &localRun{
		ref:    "run:" + req.TaskID,
		events: make(chan Event, 32),
		cancel: cancel,
		done:   make(chan struct{}),
	}
	go r.execute(ctx, req, run)
	return run, nil
}

func (r *LocalRunner) maxSteps() int {
	if r.cfg.MaxSteps <= 0 {
		return defaultMaxSteps
	}
	return r.cfg.MaxSteps
}

type localRun struct {
	ref    string
	events chan Event
	cancel context.CancelFunc
	done   chan struct{}

	// mu 保护 stopped：stop 设置它，emit 检查它。二者互斥，于是「已取消的
	// 执行不会再上报终态」在事件缓冲区还有空间时是确定的（见 emit）。
	mu      sync.Mutex
	stopped bool
}

func (r *localRun) Events() <-chan Event { return r.events }
func (r *localRun) Ref() string          { return r.ref }

// stop 标记执行已停止并取消其 ctx。幂等。
func (r *localRun) stop() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
	r.cancel()
}

// Interrupt 在进程内 loop 中退化为取消整个执行：单轮任务没有 steer 语义。
func (r *localRun) Interrupt(context.Context) error {
	r.stop()
	return nil
}

func (r *localRun) Close() error {
	r.stop()
	<-r.done
	return nil
}

func (r *LocalRunner) execute(ctx context.Context, req RunRequest, run *localRun) {
	defer close(run.events)
	defer close(run.done)
	defer run.stop()

	emit := func(event Event) bool {
		// 取消优先：执行已停止时不再上报终态，否则被取消的任务会被误判成
		// 正常完成。缓冲区有空间时，发送在锁内完成，与 stop 互斥——stop
		// 返回之后不可能再送出终态事件。
		run.mu.Lock()
		if run.stopped || ctx.Err() != nil {
			run.mu.Unlock()
			return false
		}
		select {
		case run.events <- event:
			run.mu.Unlock()
			return true
		default:
		}
		run.mu.Unlock()
		// 缓冲区满：等消费方腾位置，期间取消会通过 ctx.Done() 生效。
		select {
		case run.events <- event:
			return true
		case <-ctx.Done():
			return false
		}
	}

	registry := r.cfg.Tools()
	if registry == nil {
		emit(Event{Type: EventError, Err: errors.New("local runner: nil tool registry")})
		return
	}
	maxSteps := r.maxSteps()
	client := llm.AdaptLegacyClient(r.cfg.Client)

	messages := []llm.Message{
		{Role: string(llm.RoleSystem), Content: taskSystemPrompt},
		{Role: string(llm.RoleUser), Content: req.Title},
	}

	var total agent.Usage
	for step := 0; step < maxSteps; step++ {
		if ctx.Err() != nil {
			return
		}
		result, err := r.step(ctx, client, registry, messages, emit)
		if result.usage != nil {
			total.Steps++
			total.InputTokens += result.usage.InputTokens
			total.OutputTokens += result.usage.OutputTokens
			total.ReasoningTokens += result.usage.ReasoningTokens
			total.CacheReadTokens += result.usage.CacheReadTokens
			total.CacheWriteTokens += result.usage.CacheWriteTokens
		}
		if err != nil {
			if isContextErr(err) {
				return
			}
			logging.Errorf("task %s: llm step: %v", req.TaskID, err)
			// 失败路径也要带上已发生的用量：这一步的 token 已经付掉了，
			// 丢掉就是少计费（与 agent.runLoop 的 error 路径同一口径）。
			emit(Event{Type: EventError, Err: err, Usage: usageOrNil(total)})
			return
		}

		messages = append(messages, llm.Message{
			Role:            string(llm.RoleAssistant),
			Content:         result.text,
			ToolCalls:       result.toolCalls,
			ProviderContext: result.providerContext,
		})
		if len(result.toolCalls) == 0 {
			emit(Event{Type: EventDone, Usage: usageOrNil(total)})
			return
		}
		for _, call := range result.toolCalls {
			if !emit(Event{Type: EventToolUse, Tool: &ToolUse{Name: call.Name, Input: call.Arguments}}) {
				return
			}
			msg, err := executeToolCall(ctx, registry, call)
			if err != nil {
				logging.Errorf("task %s: tool %s: %v", req.TaskID, call.Name, err)
				emit(Event{Type: EventError, Err: err})
				return
			}
			messages = append(messages, msg)
		}
	}
	emit(Event{Type: EventError, Err: fmt.Errorf("reached max steps (%d)", maxSteps)})
}

// localStepResult 是单轮 LLM 调用的结果，与 agent.stepResult 同形但
// 不携带对话会话。
type localStepResult struct {
	text            string
	toolCalls       []llm.ToolCall
	providerContext *llm.ProviderContext
	usage           *llm.Usage
}

// step 执行一次 LLM 调用并消费其流式响应：文本增量通过 emit 发送
// EventText，工具调用与用量随返回值带出。
func (r *LocalRunner) step(
	ctx context.Context,
	client llm.GenerationClient,
	registry *tools.Registry,
	messages []llm.Message,
	emit func(Event) bool,
) (localStepResult, error) {
	req := llm.Request{
		Messages:        messages,
		Tools:           registry.Definitions(),
		Thinking:        r.cfg.Thinking,
		ProviderOptions: append([]byte(nil), r.cfg.Options...),
	}
	if r.cfg.MaxOutputTokens > 0 {
		req.MaxOutputTokens = &r.cfg.MaxOutputTokens
	}
	stream, err := client.Stream(ctx, req)
	if err != nil {
		return localStepResult{}, err
	}
	defer func() { _ = stream.Close() }()

	var fullText, bufferedContent string
	lastFilteredLength := 0
	var finalResponse *llm.Response

	for {
		event, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			// 流中途报错：响应可能已经带回了用量，保留它，别让已发生的
			// token 在错误路径上消失。
			return localStepResult{text: fullText, usage: responseUsage(finalResponse)}, err
		}
		switch event.Type {
		case llm.EventTextDelta:
			bufferedContent += event.TextDelta
			newContent, nextLength := deltaFromBufferedContent(bufferedContent, lastFilteredLength)
			if newContent != "" {
				if !emit(Event{Type: EventText, Text: newContent}) {
					return localStepResult{}, ctx.Err()
				}
				fullText += newContent
			}
			lastFilteredLength = nextLength
		case llm.EventResponseDone:
			finalResponse = event.Response
		}
	}

	result := localStepResult{text: fullText}
	if finalResponse == nil {
		return result, nil
	}
	usage := finalResponse.Usage
	result.usage = &usage
	if result.text == "" {
		result.text = finalResponse.Message.Text()
	}
	result.toolCalls = finalResponse.Message.Calls()
	result.providerContext = finalResponse.Message.ProviderContext
	return result, nil
}

// deltaFromBufferedContent 从累积 buffer 中提取自 lastLength 之后的新增量。
// 与 agent.step 的同名逻辑保持一致：部分 provider 会整段重发累积文本。
func deltaFromBufferedContent(content string, lastLength int) (string, int) {
	if lastLength < 0 {
		lastLength = 0
	}
	if lastLength > len(content) {
		lastLength = len(content)
	}
	return content[lastLength:], len(content)
}

// executeToolCall 执行单个工具调用。未注册工具生成可恢复的 tool 错误
// 消息；Execute 返回的 Go error 视为框架级故障，终止整个执行。
func executeToolCall(ctx context.Context, registry *tools.Registry, call llm.ToolCall) (llm.Message, error) {
	if !registry.CanExecute(call.Name) {
		logging.Warnf("task: unknown tool: %s", call.Name)
		return toolResultMessage(call.ID, fmt.Sprintf("error: unknown tool %q", call.Name)), nil
	}
	result, err := registry.Execute(ctx, call.Name, json.RawMessage(call.Arguments))
	if err != nil {
		return llm.Message{}, err
	}
	content := result.Output
	if result.Error != nil {
		logging.Errorf("task: tool %s error: %v", call.Name, result.Error)
		if content == "" {
			content = result.Error.Error()
		}
	}
	return toolResultMessage(call.ID, content), nil
}

func toolResultMessage(id, content string) llm.Message {
	return llm.Message{Role: string(llm.RoleTool), ToolCallID: id, Content: content}
}

func usageOrNil(usage agent.Usage) *agent.Usage {
	if usage.IsZero() {
		return nil
	}
	return &usage
}

// responseUsage 取出响应里的用量；响应为空时返回 nil。
func responseUsage(resp *llm.Response) *llm.Usage {
	if resp == nil {
		return nil
	}
	usage := resp.Usage
	return &usage
}

func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
