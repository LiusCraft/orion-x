package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/llm/provider"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/respjson"
	"github.com/openai/openai-go/v3/shared"
)

type Config struct {
	APIKey          string
	BaseURL         string
	Model           string
	Dialect         string
	Scope           string
	Options         []byte
	ExtraFields     map[string]any
	Thinking        llm.ThinkingConfig
	MaxOutputTokens int
}

type chatAdapter struct {
	client openai.Client
	cfg    Config
}

type dialectState struct {
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ReasoningDetails json.RawMessage `json:"reasoning_details,omitempty"`
}

func init() {
	constructor := func(ctx context.Context, cfg provider.Config) (provider.Adapter, error) {
		return New(ctx, Config{
			APIKey:          cfg.APIKey,
			BaseURL:         cfg.BaseURL,
			Model:           cfg.Model,
			Dialect:         cfg.Dialect,
			Scope:           cfg.Scope,
			Options:         cfg.Options,
			ExtraFields:     cfg.ExtraFields,
			Thinking:        cfg.Thinking,
			MaxOutputTokens: cfg.MaxOutputTokens,
		})
	}
	meta := provider.ProviderMeta{
		Name:           "OpenAI Chat Completions",
		DefaultBaseURL: "https://api.openai.com/v1",
	}
	provider.Register("openai-completions", constructor, meta)
	provider.Register("openai", constructor, meta)
}

func New(_ context.Context, cfg Config) (provider.Adapter, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("openai api key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("openai model is required")
	}
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(strings.TrimRight(cfg.BaseURL, "/")))
	}
	return &chatAdapter{client: openai.NewClient(opts...), cfg: cfg}, nil
}

func (c *chatAdapter) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	params, err := c.params(req)
	if err != nil {
		return llm.Response{}, err
	}
	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return llm.Response{}, mapError("openai-completions", err)
	}
	if len(completion.Choices) == 0 {
		return llm.Response{Model: c.cfg.Model, Message: llm.Message{Role: string(llm.RoleAssistant)}}, nil
	}
	choice := completion.Choices[0]
	message := c.messageFromSDK(choice.Message)
	return llm.Response{
		ID:         completion.ID,
		Model:      completion.Model,
		Message:    message,
		StopReason: mapFinishReason(choice.FinishReason),
		StopDetail: string(choice.FinishReason),
		Usage:      llm.Usage{InputTokens: int64(completion.Usage.PromptTokens), OutputTokens: int64(completion.Usage.CompletionTokens), TotalTokens: int64(completion.Usage.TotalTokens)},
	}, nil
}

func (c *chatAdapter) Stream(ctx context.Context, req llm.Request) (llm.Stream, error) {
	params, err := c.params(req)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream := c.client.Chat.Completions.NewStreaming(streamCtx, params)
	out := llm.NewEventStream(cancel)
	go func() {
		defer out.Finish()
		out.Send(llm.Event{Type: llm.EventResponseStart})
		acc := openai.ChatCompletionAccumulator{}
		state := dialectState{}
		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) == 0 {
				continue
			}
			delta := chunk.Choices[0].Delta
			if reasoning := extractReasoningDelta(delta.JSON.ExtraFields, &state); reasoning != "" {
				if !out.Send(llm.Event{Type: llm.EventReasoningSummaryDelta, Reasoning: reasoning}) {
					return
				}
			}
			acc.AddChunk(chunk)
			if delta.Content != "" {
				if !out.Send(llm.Event{Type: llm.EventTextDelta, TextDelta: delta.Content}) {
					return
				}
			}
		}
		if err := stream.Err(); err != nil {
			out.SendError(mapError("openai-completions", err))
			return
		}
		if len(acc.Choices) == 0 {
			out.Send(llm.Event{Type: llm.EventResponseDone, Response: &llm.Response{Model: c.cfg.Model}})
			return
		}
		choice := acc.Choices[0]
		message := c.messageFromSDK(choice.Message)
		if len(state.ReasoningContent) > 0 || len(state.ReasoningDetails) > 0 {
			message.ProviderContext = c.providerContext(state)
		}
		resp := llm.Response{
			ID:         acc.ID,
			Model:      acc.Model,
			Message:    message,
			StopReason: mapFinishReason(choice.FinishReason),
			StopDetail: string(choice.FinishReason),
		}
		out.Send(llm.Event{Type: llm.EventResponseDone, Response: &resp})
	}()
	return out, nil
}

func (c *chatAdapter) params(req llm.Request) (openai.ChatCompletionNewParams, error) {
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.cfg.Model),
		Messages: convertMessages(req.Messages),
		Tools:    convertTools(req.Tools),
	}
	if req.MaxOutputTokens != nil {
		params.MaxCompletionTokens = param.NewOpt(int64(*req.MaxOutputTokens))
	} else if c.cfg.MaxOutputTokens > 0 {
		params.MaxCompletionTokens = param.NewOpt(int64(c.cfg.MaxOutputTokens))
	}
	if req.Temperature != nil {
		params.Temperature = param.NewOpt(*req.Temperature)
	}
	if req.ParallelTools != nil {
		params.ParallelToolCalls = param.NewOpt(*req.ParallelTools)
	}
	if req.ToolChoice.Mode != "" {
		params.ToolChoice = convertToolChoice(req.ToolChoice)
	}
	applyRawExtraFields(&params, c.cfg.ExtraFields)
	if err := applyJSONExtraFields(&params, c.cfg.Options); err != nil {
		return params, err
	}
	if err := applyJSONExtraFields(&params, req.ProviderOptions); err != nil {
		return params, err
	}
	thinking := req.Thinking
	if thinking.IsDefault() {
		thinking = c.cfg.Thinking
	}
	if err := applyDialect(&params, c.cfg.Dialect, c.cfg.Model, thinking, req.ProviderOptions); err != nil {
		return params, err
	}
	return params, nil
}

func convertMessages(messages []llm.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, raw := range messages {
		msg := raw.Normalize()
		switch msg.Role {
		case string(llm.RoleSystem):
			result = append(result, openai.SystemMessage(msg.Text()))
		case string(llm.RoleUser):
			for _, block := range msg.Blocks {
				if block.Type == llm.BlockTypeToolResult && block.ToolResult != nil {
					result = append(result, openai.ToolMessage(block.ToolResult.Content, block.ToolResult.ToolCallID))
				} else if block.Type == llm.BlockTypeText {
					result = append(result, openai.UserMessage(block.Text))
				}
			}
		case string(llm.RoleTool):
			result = append(result, openai.ToolMessage(msg.Content, msg.ToolCallID))
		case string(llm.RoleAssistant):
			assistant := openai.ChatCompletionAssistantMessageParam{}
			if text := msg.Text(); text != "" {
				assistant.Content.OfString = param.NewOpt(text)
			}
			for _, call := range msg.Calls() {
				assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: call.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name: call.Name, Arguments: call.Arguments,
						},
					},
				})
			}
			applyProviderContext(&assistant, msg.ProviderContext)
			result = append(result, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
		}
	}
	return result
}

func convertTools(defs []llm.ToolDefinition) []openai.ChatCompletionToolUnionParam {
	if len(defs) == 0 {
		return nil
	}
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(defs))
	for _, def := range defs {
		schema, err := def.Schema()
		if err != nil {
			continue
		}
		var params map[string]any
		_ = json.Unmarshal(schema, &params)
		result = append(result, openai.ChatCompletionToolUnionParam{OfFunction: &openai.ChatCompletionFunctionToolParam{
			Function: openai.FunctionDefinitionParam{Name: def.Name, Description: openai.String(def.Description), Parameters: openai.FunctionParameters(params)},
		}})
	}
	return result
}

func convertToolChoice(choice llm.ToolChoice) openai.ChatCompletionToolChoiceOptionUnionParam {
	switch choice.Mode {
	case llm.ToolChoiceNone:
		return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoNone))}
	case llm.ToolChoiceRequired:
		return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoRequired))}
	case llm.ToolChoiceFunction:
		return openai.ToolChoiceOptionFunctionToolChoice(openai.ChatCompletionNamedToolChoiceFunctionParam{Name: choice.Name})
	default:
		return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.NewOpt(string(openai.ChatCompletionToolChoiceOptionAutoAuto))}
	}
}

func (c *chatAdapter) messageFromSDK(message openai.ChatCompletionMessage) llm.Message {
	msg := llm.Message{Role: string(llm.RoleAssistant), Content: message.Content}
	for _, call := range message.ToolCalls {
		if function, ok := call.AsAny().(openai.ChatCompletionMessageFunctionToolCall); ok {
			msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{ID: function.ID, Name: function.Function.Name, Arguments: function.Function.Arguments})
		}
	}
	msg.Blocks = append(msg.Blocks, llm.Block{Type: llm.BlockTypeText, Text: message.Content})
	for _, call := range msg.ToolCalls {
		callCopy := call
		msg.Blocks = append(msg.Blocks, llm.Block{Type: llm.BlockTypeToolCall, ToolCall: &callCopy})
	}
	return msg
}

func (c *chatAdapter) providerContext(state dialectState) *llm.ProviderContext {
	data, _ := json.Marshal(state)
	return &llm.ProviderContext{Adapter: "openai-completions", Model: c.cfg.Model, Scope: c.cfg.Scope, Data: data}
}

func applyRawExtraFields(params interface{ SetExtraFields(map[string]any) }, fields map[string]any) {
	if len(fields) > 0 {
		params.SetExtraFields(fields)
	}
}

func applyJSONExtraFields(params interface{ SetExtraFields(map[string]any) }, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("provider options must be a JSON object: %w", err)
	}
	params.SetExtraFields(fields)
	return nil
}

func applyProviderContext(params interface{ SetExtraFields(map[string]any) }, context *llm.ProviderContext) {
	if context == nil || len(context.Data) == 0 {
		return
	}
	var state dialectState
	if json.Unmarshal(context.Data, &state) == nil {
		fields := map[string]any{}
		if state.ReasoningContent != "" {
			fields["reasoning_content"] = state.ReasoningContent
		}
		if len(state.ReasoningDetails) > 0 {
			var details any
			if json.Unmarshal(state.ReasoningDetails, &details) == nil {
				fields["reasoning_details"] = details
			}
		}
		params.SetExtraFields(fields)
	}
}

func applyDialect(params *openai.ChatCompletionNewParams, dialect, model string, thinking llm.ThinkingConfig, raw []byte) error {
	dialect = strings.ToLower(strings.TrimSpace(dialect))
	if dialect == "" || dialect == "openai" || dialect == "generic" {
		return nil
	}
	fields := map[string]any{}
	switch dialect {
	case "minimax":
		if thinking.Mode == llm.ThinkingModeDisabled && strings.HasPrefix(strings.ToLower(model), "minimax-m2") {
			return &llm.UnsupportedOptionError{Adapter: "minimax", Option: "thinking.disabled", Reason: "M2.x thinking cannot be disabled"}
		}
		switch thinking.Mode {
		case llm.ThinkingModeEnabled:
			fields["thinking"] = map[string]string{"type": "adaptive"}
		case llm.ThinkingModeDisabled:
			fields["thinking"] = map[string]string{"type": "disabled"}
		}
		if thinking.ExposeSummary {
			fields["reasoning_split"] = true
		}
	case "deepseek":
		switch thinking.Mode {
		case llm.ThinkingModeEnabled:
			fields["thinking"] = map[string]string{"type": "enabled"}
		case llm.ThinkingModeDisabled:
			fields["thinking"] = map[string]string{"type": "disabled"}
		}
		if thinking.HasEffort() {
			effort := string(thinking.Effort)
			if effort == "low" || effort == "medium" {
				effort = "high"
			}
			if effort == "xhigh" {
				effort = "max"
			}
			if effort != "high" && effort != "max" {
				return &llm.UnsupportedOptionError{Adapter: "deepseek", Option: "thinking.effort", Reason: "supported values are high and max"}
			}
			params.ReasoningEffort = shared.ReasoningEffort(effort)
		}
	case "qwen":
		switch thinking.Mode {
		case llm.ThinkingModeEnabled:
			fields["enable_thinking"] = true
		case llm.ThinkingModeDisabled:
			fields["enable_thinking"] = false
		}
		if thinking.BudgetTokens != nil {
			fields["thinking_budget"] = *thinking.BudgetTokens
		}
		if thinking.PreserveHistory == llm.PreserveModeAll {
			fields["preserve_thinking"] = true
		}
	case "kimi":
		lowerModel := strings.ToLower(model)
		if strings.Contains(lowerModel, "k3") || strings.Contains(lowerModel, "k2.7-code") {
			if thinking.Mode == llm.ThinkingModeDisabled {
				return &llm.UnsupportedOptionError{Adapter: "kimi", Option: "thinking.disabled", Reason: "this model always thinks"}
			}
			if thinking.HasEffort() && strings.Contains(lowerModel, "k3") {
				fields["reasoning_effort"] = string(thinking.Effort)
			}
		} else if strings.Contains(lowerModel, "k2.6") || strings.Contains(lowerModel, "k2.5") {
			thinkingObj := map[string]any{}
			switch thinking.Mode {
			case llm.ThinkingModeEnabled:
				thinkingObj["type"] = "enabled"
			case llm.ThinkingModeDisabled:
				thinkingObj["type"] = "disabled"
			}
			if strings.Contains(lowerModel, "k2.6") && thinking.PreserveHistory == llm.PreserveModeAll {
				thinkingObj["keep"] = "all"
			}
			if len(thinkingObj) > 0 {
				fields["thinking"] = thinkingObj
			}
		}
	default:
		return nil
	}
	if len(raw) > 0 {
		var options map[string]any
		if err := json.Unmarshal(raw, &options); err != nil {
			return fmt.Errorf("%s provider options: %w", dialect, err)
		}
		for k, value := range options {
			fields[k] = value
		}
	}
	if len(fields) > 0 {
		params.SetExtraFields(fields)
	}
	return nil
}

func extractReasoningDelta(fields map[string]respjson.Field, state *dialectState) string {
	if fields == nil {
		return ""
	}
	var raw string
	if field, ok := fields["reasoning_content"]; ok {
		raw = fieldString(field)
	}
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, state.ReasoningContent) {
		delta := strings.TrimPrefix(raw, state.ReasoningContent)
		state.ReasoningContent = raw
		return delta
	}
	state.ReasoningContent += raw
	return raw
}

func fieldString(field respjson.Field) string {
	var s string
	_ = json.Unmarshal([]byte(field.Raw()), &s)
	return s
}

func mapFinishReason(reason string) llm.StopReason {
	switch reason {
	case "stop":
		return llm.StopReasonStop
	case "length":
		return llm.StopReasonLength
	case "tool_calls", "function_call":
		return llm.StopReasonToolCalls
	case "content_filter":
		return llm.StopReasonContentFilter
	default:
		return llm.StopReasonUnknown
	}
}

func mapError(adapter string, err error) error {
	if err == nil {
		return nil
	}
	return &llm.APIError{Adapter: adapter, Message: err.Error(), Cause: err}
}

var _ provider.Adapter = (*chatAdapter)(nil)
