package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/llm/provider"
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
	client anthropic.Client
	cfg    Config
}

type contextData struct {
	Content json.RawMessage `json:"content"`
}

func init() {
	provider.Register("anthropic-messages", func(ctx context.Context, cfg provider.Config) (provider.Adapter, error) {
		return New(ctx, Config{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model, Scope: cfg.Scope, Options: cfg.Options, Thinking: cfg.Thinking, MaxOutputTokens: cfg.MaxOutputTokens})
	}, provider.ProviderMeta{Name: "Anthropic Messages", DefaultBaseURL: "https://api.anthropic.com"})
}

func New(_ context.Context, cfg Config) (provider.Adapter, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("anthropic api key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("anthropic model is required")
	}
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(strings.TrimRight(cfg.BaseURL, "/")))
	}
	return &adapter{client: anthropic.NewClient(opts...), cfg: cfg}, nil
}

func (a *adapter) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	params, err := a.params(req)
	if err != nil {
		return llm.Response{}, err
	}
	message, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return llm.Response{}, &llm.APIError{Adapter: "anthropic-messages", Message: err.Error(), Cause: err}
	}
	return a.convertMessage(message), nil
}

func (a *adapter) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	params, err := a.params(req)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	sdkStream := a.client.Messages.NewStreaming(streamCtx, params)
	out := llm.NewEventStream(cancel)
	go func() {
		defer out.Finish()
		out.Send(llm.Event{Type: llm.EventResponseStart})
		var accumulated anthropic.Message
		for sdkStream.Next() {
			event := sdkStream.Current()
			if err := accumulated.Accumulate(event); err != nil {
				out.SendError(&llm.APIError{Adapter: "anthropic-messages", Message: err.Error(), Cause: err})
				return
			}
			if event.Type != "content_block_delta" {
				continue
			}
			delta := event.AsContentBlockDelta().Delta
			switch delta.Type {
			case "text_delta":
				if !out.Send(llm.Event{Type: llm.EventTextDelta, Index: int(event.Index), TextDelta: delta.Text}) {
					return
				}
			case "input_json_delta":
				if !out.Send(llm.Event{Type: llm.EventToolCallDelta, Index: int(event.Index), ToolCall: &llm.ToolCallDelta{ArgumentsDelta: delta.PartialJSON}}) {
					return
				}
			case "thinking_delta":
				if !out.Send(llm.Event{Type: llm.EventReasoningSummaryDelta, Index: int(event.Index), Reasoning: delta.Thinking}) {
					return
				}
			}
		}
		if err := sdkStream.Err(); err != nil {
			out.SendError(&llm.APIError{Adapter: "anthropic-messages", Message: err.Error(), Cause: err})
			return
		}
		resp := a.convertMessage(&accumulated)
		out.Send(llm.Event{Type: llm.EventResponseDone, Response: &resp})
	}()
	return out, nil
}

func (a *adapter) params(req llm.Request) (anthropic.MessageNewParams, error) {
	maxTokens := a.cfg.MaxOutputTokens
	if req.MaxOutputTokens != nil {
		maxTokens = *req.MaxOutputTokens
	}
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, raw := range req.Messages {
		msg := raw.Normalize()
		if msg.Role == string(llm.RoleSystem) {
			continue
		}
		role := msg.Role
		if role == string(llm.RoleTool) {
			role = string(llm.RoleUser)
		}
		var content []any
		if msg.ProviderContext != nil && role == string(llm.RoleAssistant) {
			var data contextData
			if json.Unmarshal(msg.ProviderContext.Data, &data) == nil && len(data.Content) > 0 {
				_ = json.Unmarshal(data.Content, &content)
			}
		}
		if len(content) == 0 {
			for _, block := range msg.Blocks {
				switch block.Type {
				case llm.BlockTypeText:
					content = append(content, map[string]any{"type": "text", "text": block.Text})
				case llm.BlockTypeToolCall:
					if block.ToolCall != nil {
						var input any
						_ = json.Unmarshal([]byte(block.ToolCall.Arguments), &input)
						content = append(content, map[string]any{"type": "tool_use", "id": block.ToolCall.ID, "name": block.ToolCall.Name, "input": input})
					}
				case llm.BlockTypeToolResult:
					if block.ToolResult != nil {
						content = append(content, map[string]any{"type": "tool_result", "tool_use_id": block.ToolResult.ToolCallID, "content": block.ToolResult.Content, "is_error": block.ToolResult.IsError})
					}
				}
			}
			if msg.Role == string(llm.RoleTool) {
				content = append(content, map[string]any{"type": "tool_result", "tool_use_id": msg.ToolCallID, "content": msg.Content})
			}
		}
		messages = append(messages, map[string]any{"role": role, "content": content})
	}
	body := map[string]any{"model": a.cfg.Model, "max_tokens": maxTokens, "messages": messages}
	var instructions []string
	for _, instruction := range req.Instructions {
		instructions = append(instructions, instruction.Text)
	}
	for _, msg := range req.Messages {
		if msg.Role == string(llm.RoleSystem) {
			instructions = append(instructions, msg.Text())
		}
	}
	if len(instructions) > 0 {
		body["system"] = strings.Join(instructions, "\n\n")
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, def := range req.Tools {
			schema, err := def.Schema()
			if err != nil {
				return anthropic.MessageNewParams{}, err
			}
			var inputSchema any
			_ = json.Unmarshal(schema, &inputSchema)
			tool := map[string]any{"name": def.Name, "description": def.Description, "input_schema": inputSchema}
			if def.SchemaMode == llm.SchemaModeStrict {
				tool["strict"] = true
			}
			tools = append(tools, tool)
		}
		body["tools"] = tools
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if len(req.StopSequences) > 0 {
		body["stop_sequences"] = req.StopSequences
	}
	thinking := req.Thinking
	if thinking.IsDefault() {
		thinking = a.cfg.Thinking
	}
	switch thinking.Mode {
	case llm.ThinkingModeEnabled:
		budget := 1024
		if thinking.BudgetTokens != nil {
			budget = *thinking.BudgetTokens
		}
		body["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budget}
	case llm.ThinkingModeDisabled:
		body["thinking"] = map[string]any{"type": "disabled"}
	}
	if len(a.cfg.Options) > 0 {
		var options map[string]any
		if err := json.Unmarshal(a.cfg.Options, &options); err != nil {
			return anthropic.MessageNewParams{}, err
		}
		for key, value := range options {
			body[key] = value
		}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	var params anthropic.MessageNewParams
	if err := json.Unmarshal(data, &params); err != nil {
		return params, fmt.Errorf("build anthropic params: %w", err)
	}
	return params, nil
}

func (a *adapter) convertMessage(message *anthropic.Message) llm.Response {
	msg := llm.Message{Role: string(llm.RoleAssistant)}
	contentRaw, _ := json.Marshal(message.Content)
	for _, block := range message.Content {
		switch block.Type {
		case "text":
			msg.Content += block.Text
			msg.Blocks = append(msg.Blocks, llm.Block{Type: llm.BlockTypeText, Text: block.Text})
		case "tool_use":
			call := llm.ToolCall{ID: block.ID, Name: block.Name, Arguments: string(block.Input)}
			msg.ToolCalls = append(msg.ToolCalls, call)
			callCopy := call
			msg.Blocks = append(msg.Blocks, llm.Block{Type: llm.BlockTypeToolCall, ToolCall: &callCopy})
		}
	}
	msg.ProviderContext = &llm.ProviderContext{Adapter: "anthropic-messages", Model: a.cfg.Model, Scope: a.cfg.Scope, Data: mustJSON(contextData{Content: contentRaw})}
	usage := llm.Usage{InputTokens: message.Usage.InputTokens, OutputTokens: message.Usage.OutputTokens}
	usage.CacheReadTokens = message.Usage.CacheReadInputTokens
	usage.CacheWriteTokens = message.Usage.CacheCreationInputTokens
	usage.TotalTokens = usage.InputTokens + usage.OutputTokens + usage.CacheReadTokens + usage.CacheWriteTokens
	return llm.Response{ID: message.ID, Model: string(message.Model), Message: msg, StopReason: mapStopReason(message.StopReason), StopDetail: string(message.StopReason), Usage: usage}
}

func mapStopReason(reason anthropic.StopReason) llm.StopReason {
	switch string(reason) {
	case "end_turn", "stop_sequence":
		return llm.StopReasonStop
	case "max_tokens", "model_context_window":
		return llm.StopReasonLength
	case "tool_use":
		return llm.StopReasonToolCalls
	case "pause_turn":
		return llm.StopReasonPause
	case "refusal":
		return llm.StopReasonContentFilter
	default:
		return llm.StopReasonUnknown
	}
}

func mustJSON(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

var _ provider.Adapter = (*adapter)(nil)
