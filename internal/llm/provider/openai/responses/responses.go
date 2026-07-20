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
	input := make([]map[string]any, 0, len(req.Messages))
	for _, raw := range req.Messages {
		msg := raw.Normalize()
		if msg.ProviderContext != nil && msg.Role == string(llm.RoleAssistant) && len(msg.ProviderContext.Data) > 0 {
			var data contextData
			if json.Unmarshal(msg.ProviderContext.Data, &data) == nil && len(data.Items) > 0 {
				for _, item := range data.Items {
					var value map[string]any
					if json.Unmarshal(item, &value) == nil {
						input = append(input, value)
					}
				}
				continue
			}
		}
		for _, block := range msg.Blocks {
			switch block.Type {
			case llm.BlockTypeText:
				input = append(input, map[string]any{"role": msg.Role, "content": block.Text})
			case llm.BlockTypeToolCall:
				if block.ToolCall != nil {
					input = append(input, map[string]any{"type": "function_call", "call_id": block.ToolCall.ID, "name": block.ToolCall.Name, "arguments": block.ToolCall.Arguments})
				}
			case llm.BlockTypeToolResult:
				if block.ToolResult != nil {
					input = append(input, map[string]any{"type": "function_call_output", "call_id": block.ToolResult.ToolCallID, "output": block.ToolResult.Content})
				}
			}
		}
		if msg.Role == string(llm.RoleTool) {
			input = append(input, map[string]any{"type": "function_call_output", "call_id": msg.ToolCallID, "output": msg.Content})
		}
	}
	body := map[string]any{"model": a.cfg.Model, "input": input, "store": false}
	if len(req.Instructions) > 0 {
		var b strings.Builder
		for i, instruction := range req.Instructions {
			if i > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(instruction.Text)
		}
		body["instructions"] = b.String()
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, def := range req.Tools {
			schema, err := def.Schema()
			if err != nil {
				return openairesponses.ResponseNewParams{}, err
			}
			var parameters any
			_ = json.Unmarshal(schema, &parameters)
			tools = append(tools, map[string]any{"type": "function", "name": def.Name, "description": def.Description, "parameters": parameters, "strict": def.SchemaMode == llm.SchemaModeStrict})
		}
		body["tools"] = tools
	}
	if req.MaxOutputTokens != nil {
		body["max_output_tokens"] = *req.MaxOutputTokens
	} else if a.cfg.MaxOutputTokens > 0 {
		body["max_output_tokens"] = a.cfg.MaxOutputTokens
	}
	if req.ParallelTools != nil {
		body["parallel_tool_calls"] = *req.ParallelTools
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	thinking := req.Thinking
	if thinking.IsDefault() {
		thinking = a.cfg.Thinking
	}
	if thinking.HasEffort() {
		body["reasoning"] = map[string]any{"effort": string(thinking.Effort)}
	}
	if provider.InferDialect(a.cfg.Model) == provider.DialectQwen && !thinking.HasEffort() && thinking.Mode == llm.ThinkingModeDisabled {
		body["reasoning"] = map[string]any{"effort": "none"}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return openairesponses.ResponseNewParams{}, err
	}
	var params openairesponses.ResponseNewParams
	if err := json.Unmarshal(data, &params); err != nil {
		return params, fmt.Errorf("build responses params: %w", err)
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
	usage := llm.Usage{}
	usage.InputTokens = int64(resp.Usage.InputTokens)
	usage.OutputTokens = int64(resp.Usage.OutputTokens)
	usage.TotalTokens = int64(resp.Usage.TotalTokens)
	usage.ReasoningTokens = int64(resp.Usage.OutputTokensDetails.ReasoningTokens)
	return llm.Response{ID: resp.ID, Model: string(resp.Model), Message: message, StopReason: stop, Usage: usage}
}

var _ provider.Adapter = (*adapter)(nil)
