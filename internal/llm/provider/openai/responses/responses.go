package responses

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/llm/provider"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	openairesponses "github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type Config struct {
	APIKey          string
	BaseURL         string
	Model           string
	Scope           string
	Options         []byte
	Thinking        llm.ThinkingConfig
	MaxOutputTokens int
}

type adapter struct {
	client openai.Client
	cfg    Config
}

type contextData struct {
	Items []json.RawMessage `json:"items"`
}

func init() {
	provider.Register("openai-responses", func(ctx context.Context, cfg provider.Config) (provider.Adapter, error) {
		return New(ctx, Config{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model, Scope: cfg.Scope, Options: cfg.Options, Thinking: cfg.Thinking, MaxOutputTokens: cfg.MaxOutputTokens})
	}, provider.ProviderMeta{Name: "OpenAI Responses", DefaultBaseURL: "https://api.openai.com/v1"})
}

func New(_ context.Context, cfg Config) (provider.Adapter, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("openai responses api key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("openai responses model is required")
	}
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(strings.TrimRight(cfg.BaseURL, "/")))
	}
	return &adapter{client: openai.NewClient(opts...), cfg: cfg}, nil
}

func (a *adapter) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	params, err := a.params(req)
	if err != nil {
		return llm.Response{}, err
	}
	resp, err := a.client.Responses.New(ctx, params)
	if err != nil {
		return llm.Response{}, &llm.APIError{Adapter: "openai-responses", Message: err.Error(), Cause: err}
	}
	return a.convertResponse(resp), nil
}

func (a *adapter) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	params, err := a.params(req)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	sdkStream := a.client.Responses.NewStreaming(streamCtx, params)
	out := llm.NewEventStream(cancel)
	go func() {
		defer out.Finish()
		out.Send(llm.Event{Type: llm.EventResponseStart})
		for sdkStream.Next() {
			event := sdkStream.Current()
			switch event.Type {
			case "response.output_text.delta":
				v := event.AsResponseOutputTextDelta()
				if !out.Send(llm.Event{Type: llm.EventTextDelta, Index: int(v.OutputIndex), TextDelta: v.Delta}) {
					return
				}
			case "response.function_call_arguments.delta":
				v := event.AsResponseFunctionCallArgumentsDelta()
				if !out.Send(llm.Event{Type: llm.EventToolCallDelta, Index: int(v.OutputIndex), ToolCall: &llm.ToolCallDelta{ID: v.ItemID, ArgumentsDelta: v.Delta}}) {
					return
				}
			case "response.function_call_arguments.done":
				v := event.AsResponseFunctionCallArgumentsDone()
				if !out.Send(llm.Event{Type: llm.EventToolCallDone, Index: int(v.OutputIndex), ToolCall: &llm.ToolCallDelta{ID: v.ItemID, Name: v.Name, ArgumentsDelta: v.Arguments, Done: true}}) {
					return
				}
			case "response.reasoning_summary_text.delta":
				v := event.AsResponseReasoningSummaryTextDelta()
				if !out.Send(llm.Event{Type: llm.EventReasoningSummaryDelta, Index: int(v.OutputIndex), Reasoning: v.Delta}) {
					return
				}
			case "response.completed":
				v := event.AsResponseCompleted()
				resp := a.convertResponse(&v.Response)
				out.Send(llm.Event{Type: llm.EventResponseDone, Response: &resp})
			case "response.failed", "response.incomplete":
				v := event.AsAny()
				out.SendError(&llm.APIError{Adapter: "openai-responses", Message: fmt.Sprintf("response ended with %s", event.Type), Cause: fmt.Errorf("%v", v)})
				return
			case "error":
				out.SendError(&llm.APIError{Adapter: "openai-responses", Message: event.RawJSON()})
				return
			}
		}
		if err := sdkStream.Err(); err != nil {
			out.SendError(&llm.APIError{Adapter: "openai-responses", Message: err.Error(), Cause: err})
		}
	}()
	return out, nil
}

func (a *adapter) params(req llm.Request) (openairesponses.ResponseNewParams, error) {
	params := openairesponses.ResponseNewParams{
		Model: shared.ResponsesModel(a.cfg.Model),
		Store: openai.Bool(false),
	}

	// ── input items ──
	// 直接构建 SDK 类型化的 ResponseInputItemUnionParam，避免 map→JSON→unmarshal 往返
	// （那会导致 Input 这个 union 字段在 SDK 重新序列化时被丢弃）。
	input := make([]openairesponses.ResponseInputItemUnionParam, 0, len(req.Messages))
	for _, raw := range req.Messages {
		msg := raw.Normalize()
		if msg.ProviderContext != nil && msg.Role == string(llm.RoleAssistant) && len(msg.ProviderContext.Data) > 0 {
			var data contextData
			if json.Unmarshal(msg.ProviderContext.Data, &data) == nil && len(data.Items) > 0 {
				restored := 0
				for _, item := range data.Items {
					if itemParam, ok := restoreProviderItem(item); ok {
						input = append(input, itemParam)
						restored++
					}
				}
				if restored > 0 {
					continue
				}
			}
		}
		// tool 角色的消息是工具执行结果：Responses API 里必须编码为 function_call_output item，
		// 不能作为 role:"tool" 的消息（DeepSeek 只认 user/assistant/system/developer）。
		// Normalize() 会把其 Content 补成 Text block，这里要跳过 Text，只发 function_call_output。
		if msg.Role == string(llm.RoleTool) {
			emitted := false
			for _, block := range msg.Blocks {
				if block.Type == llm.BlockTypeToolResult && block.ToolResult != nil {
					input = append(input, openairesponses.ResponseInputItemParamOfFunctionCallOutput(block.ToolResult.ToolCallID, block.ToolResult.Content))
					emitted = true
				}
			}
			if !emitted && msg.ToolCallID != "" {
				input = append(input, openairesponses.ResponseInputItemParamOfFunctionCallOutput(msg.ToolCallID, msg.Content))
			}
			continue
		}
		for _, block := range msg.Blocks {
			switch block.Type {
			case llm.BlockTypeText:
				input = append(input, openairesponses.ResponseInputItemParamOfMessage(block.Text, openairesponses.EasyInputMessageRole(msg.Role)))
			case llm.BlockTypeToolCall:
				if block.ToolCall != nil {
					input = append(input, openairesponses.ResponseInputItemParamOfFunctionCall(block.ToolCall.Arguments, block.ToolCall.ID, block.ToolCall.Name))
				}
			case llm.BlockTypeToolResult:
				if block.ToolResult != nil {
					input = append(input, openairesponses.ResponseInputItemParamOfFunctionCallOutput(block.ToolResult.ToolCallID, block.ToolResult.Content))
				}
			}
		}
	}
	if len(input) > 0 {
		params.Input = openairesponses.ResponseNewParamsInputUnion{OfInputItemList: input}
	}

	// ── instructions ──
	if len(req.Instructions) > 0 {
		var b strings.Builder
		for i, instruction := range req.Instructions {
			if i > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(instruction.Text)
		}
		params.Instructions = openai.String(b.String())
	}

	// ── tools ──
	if len(req.Tools) > 0 {
		tools := make([]openairesponses.ToolUnionParam, 0, len(req.Tools))
		for _, def := range req.Tools {
			schema, err := def.Schema()
			if err != nil {
				return openairesponses.ResponseNewParams{}, err
			}
			var parameters map[string]any
			_ = json.Unmarshal(schema, &parameters)
			tools = append(tools, openairesponses.ToolUnionParam{OfFunction: &openairesponses.FunctionToolParam{
				Name:        def.Name,
				Description: openai.String(def.Description),
				Parameters:  parameters,
				Strict:      openai.Bool(def.SchemaMode == llm.SchemaModeStrict),
			}})
		}
		params.Tools = tools
	}

	if req.MaxOutputTokens != nil {
		params.MaxOutputTokens = openai.Int(int64(*req.MaxOutputTokens))
	} else if a.cfg.MaxOutputTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(a.cfg.MaxOutputTokens))
	}
	if req.ParallelTools != nil {
		params.ParallelToolCalls = openai.Bool(*req.ParallelTools)
	}
	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}

	thinking := req.Thinking
	if thinking.IsDefault() {
		thinking = a.cfg.Thinking
	}
	if thinking.HasEffort() {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(string(thinking.Effort))}
	}
	if provider.InferDialect(a.cfg.Model) == provider.DialectQwen && !thinking.HasEffort() && thinking.Mode == llm.ThinkingModeDisabled {
		params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffortNone}
	}

	if len(a.cfg.Options) > 0 {
		var fields map[string]any
		if err := json.Unmarshal(a.cfg.Options, &fields); err != nil {
			return params, err
		}
		params.SetExtraFields(fields)
	}
	return params, nil
}

// restoreProviderItem 把上一轮响应中缓存的 output item（原始 JSON）还原为本次请求的 input item，
// 以保住 stateless 多轮对话中的 assistant 消息与工具调用上下文。
// 只还原 message / function_call / function_call_output，其余类型（reasoning 等）跳过。
func restoreProviderItem(raw json.RawMessage) (openairesponses.ResponseInputItemUnionParam, bool) {
	var item struct {
		Type      string          `json:"type"`
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		CallID    string          `json:"call_id"`
		Name      string          `json:"name"`
		Arguments string          `json:"arguments"`
		Output    string          `json:"output"`
	}
	if json.Unmarshal(raw, &item) != nil {
		return openairesponses.ResponseInputItemUnionParam{}, false
	}
	switch item.Type {
	case "message":
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		var text string
		if json.Unmarshal(item.Content, &parts) == nil {
			for _, p := range parts {
				if p.Type == "output_text" {
					text += p.Text
				}
			}
		}
		if text == "" {
			return openairesponses.ResponseInputItemUnionParam{}, false
		}
		role := openairesponses.EasyInputMessageRole(item.Role)
		if role == "" {
			role = openairesponses.EasyInputMessageRoleAssistant
		}
		return openairesponses.ResponseInputItemParamOfMessage(text, role), true
	case "function_call":
		if item.CallID == "" || item.Name == "" {
			return openairesponses.ResponseInputItemUnionParam{}, false
		}
		return openairesponses.ResponseInputItemParamOfFunctionCall(item.Arguments, item.CallID, item.Name), true
	case "function_call_output":
		if item.CallID == "" {
			return openairesponses.ResponseInputItemUnionParam{}, false
		}
		return openairesponses.ResponseInputItemParamOfFunctionCallOutput(item.CallID, item.Output), true
	default:
		return openairesponses.ResponseInputItemUnionParam{}, false
	}
}

func (a *adapter) convertResponse(resp *openairesponses.Response) llm.Response {
	message := llm.Message{Role: string(llm.RoleAssistant)}
	var contextItems []json.RawMessage
	stop := llm.StopReasonStop
	for _, item := range resp.Output {
		raw := json.RawMessage(item.RawJSON())
		contextItems = append(contextItems, raw)
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" {
					message.Content += content.Text
					message.Blocks = append(message.Blocks, llm.Block{Type: llm.BlockTypeText, Text: content.Text})
				}
			}
		case "function_call":
			args := item.Arguments.OfString
			call := llm.ToolCall{ID: item.CallID, Name: item.Name, Arguments: args}
			message.ToolCalls = append(message.ToolCalls, call)
			callCopy := call
			message.Blocks = append(message.Blocks, llm.Block{Type: llm.BlockTypeToolCall, ToolCall: &callCopy})
			stop = llm.StopReasonToolCalls
		}
	}
	if len(contextItems) > 0 {
		data, _ := json.Marshal(contextData{Items: contextItems})
		message.ProviderContext = &llm.ProviderContext{Adapter: "openai-responses", Model: a.cfg.Model, Scope: a.cfg.Scope, Data: data}
	}
	usage := provider.NormalizeUsage(provider.UsageParts{
		InputTokens:     int64(resp.Usage.InputTokens),
		OutputTokens:    int64(resp.Usage.OutputTokens),
		TotalTokens:     int64(resp.Usage.TotalTokens),
		CacheReadTokens: int64(resp.Usage.InputTokensDetails.CachedTokens),
		ReasoningTokens: int64(resp.Usage.OutputTokensDetails.ReasoningTokens),
	})
	return llm.Response{ID: resp.ID, Model: string(resp.Model), Message: message, StopReason: stop, Usage: usage}
}

var _ provider.Adapter = (*adapter)(nil)
