package toolvalidator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	einoTool "github.com/cloudwego/eino/components/tool"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	toolruntime "github.com/liuscraft/orion-x/internal/tools"
)

var ErrInvalidArgument = errors.New("invalid argument")

const defaultProbeTimeoutMs = 30000

type MCPConfigValidator struct{}

func NewMCPConfigValidator() MCPConfigValidator {
	return MCPConfigValidator{}
}

func (v MCPConfigValidator) Validate(ctx context.Context, protocol contracts.ToolProtocol, raw json.RawMessage) (json.RawMessage, error) {
	switch protocol {
	case contracts.ToolProtocolMCP:
		return validateAndProbeMCPConfig(ctx, raw)
	default:
		return nil, fmt.Errorf("%w: unsupported protocol %q", ErrInvalidArgument, protocol)
	}
}

func validateAndProbeMCPConfig(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	normalized, payload, err := normalizeMCPConfig(raw)
	if err != nil {
		return nil, err
	}

	probeConfig, requiredTools, err := buildProbeConfig(payload)
	if err != nil {
		return nil, err
	}

	if err := probeMCPServer(ctx, probeConfig, requiredTools); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidArgument, err.Error())
	}

	return normalized, nil
}

func normalizeMCPConfig(raw json.RawMessage) (json.RawMessage, map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil, fmt.Errorf("%w: config is required", ErrInvalidArgument)
	}

	var payload map[string]any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, nil, fmt.Errorf("%w: config must be valid json", ErrInvalidArgument)
	}
	if payload == nil {
		return nil, nil, fmt.Errorf("%w: config must be json object", ErrInvalidArgument)
	}

	transport, err := requireStringField(payload, "transport")
	if err != nil {
		return nil, nil, err
	}
	transport = strings.ToLower(strings.TrimSpace(transport))
	switch transport {
	case "stdio", "sse", "stream_http":
		payload["transport"] = transport
	default:
		return nil, nil, fmt.Errorf("%w: unsupported transport %q", ErrInvalidArgument, transport)
	}

	if timeoutRaw, exists := payload["timeout_ms"]; exists {
		timeout, parseErr := parsePositiveInteger(timeoutRaw)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("%w: timeout_ms must be a positive integer", ErrInvalidArgument)
		}
		payload["timeout_ms"] = timeout
	}

	if toolNamesRaw, exists := payload["tool_name_list"]; exists {
		toolNames, parseErr := parseStringArray(toolNamesRaw)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("%w: tool_name_list must be string array", ErrInvalidArgument)
		}
		payload["tool_name_list"] = toolNames
	}

	authPayload := map[string]any{"type": "none"}
	if authRaw, exists := payload["auth"]; exists {
		parsedAuthPayload, parseErr := parseObject(authRaw, "auth")
		if parseErr != nil {
			return nil, nil, parseErr
		}
		authPayload = parsedAuthPayload
	}

	authType := "none"
	if typeRaw, ok := authPayload["type"]; ok {
		parsedType, stringErr := parseOptionalString(typeRaw)
		if stringErr != nil {
			return nil, nil, fmt.Errorf("%w: auth.type must be string", ErrInvalidArgument)
		}
		authType = strings.ToLower(strings.TrimSpace(parsedType))
	}
	switch authType {
	case "none", "bearer", "api_key":
	default:
		return nil, nil, fmt.Errorf("%w: auth.type %q is not supported", ErrInvalidArgument, authType)
	}
	authPayload["type"] = authType

	if tokenRaw, ok := authPayload["token"]; ok {
		token, stringErr := parseOptionalString(tokenRaw)
		if stringErr != nil {
			return nil, nil, fmt.Errorf("%w: auth.token must be string", ErrInvalidArgument)
		}
		authPayload["token"] = strings.TrimSpace(token)
	}

	if headerRaw, ok := authPayload["header"]; ok {
		header, stringErr := parseOptionalString(headerRaw)
		if stringErr != nil {
			return nil, nil, fmt.Errorf("%w: auth.header must be string", ErrInvalidArgument)
		}
		authPayload["header"] = strings.TrimSpace(header)
	}
	payload["auth"] = authPayload

	switch transport {
	case "stdio":
		stdioPayload, parseErr := parseObject(payload["stdio"], "stdio")
		if parseErr != nil {
			return nil, nil, parseErr
		}
		command, commandErr := requireStringField(stdioPayload, "command")
		if commandErr != nil {
			return nil, nil, commandErr
		}
		stdioPayload["command"] = strings.TrimSpace(command)
		if argsRaw, exists := stdioPayload["args"]; exists {
			args, argsErr := parseStringArray(argsRaw)
			if argsErr != nil {
				return nil, nil, fmt.Errorf("%w: stdio.args must be string array", ErrInvalidArgument)
			}
			stdioPayload["args"] = args
		}
		if envRaw, exists := stdioPayload["env"]; exists {
			env, envErr := parseStringMap(envRaw, "stdio.env")
			if envErr != nil {
				return nil, nil, envErr
			}
			stdioPayload["env"] = env
		}
		if cwdRaw, exists := stdioPayload["cwd"]; exists {
			cwd, cwdErr := parseOptionalString(cwdRaw)
			if cwdErr != nil {
				return nil, nil, fmt.Errorf("%w: stdio.cwd must be string", ErrInvalidArgument)
			}
			stdioPayload["cwd"] = strings.TrimSpace(cwd)
		}
		payload["stdio"] = stdioPayload
	case "sse":
		ssePayload, parseErr := parseObject(payload["sse"], "sse")
		if parseErr != nil {
			return nil, nil, parseErr
		}
		endpoint, endpointErr := requireStringField(ssePayload, "endpoint")
		if endpointErr != nil {
			return nil, nil, endpointErr
		}
		normalizedEndpoint, normalizeErr := normalizeEndpoint(endpoint)
		if normalizeErr != nil {
			return nil, nil, normalizeErr
		}
		ssePayload["endpoint"] = normalizedEndpoint
		if headersRaw, exists := ssePayload["headers"]; exists {
			headers, headersErr := parseStringMap(headersRaw, "sse.headers")
			if headersErr != nil {
				return nil, nil, headersErr
			}
			ssePayload["headers"] = headers
		}
		payload["sse"] = ssePayload
	case "stream_http":
		streamPayload, parseErr := parseObject(payload["stream_http"], "stream_http")
		if parseErr != nil {
			return nil, nil, parseErr
		}
		endpoint, endpointErr := requireStringField(streamPayload, "endpoint")
		if endpointErr != nil {
			return nil, nil, endpointErr
		}
		normalizedEndpoint, normalizeErr := normalizeEndpoint(endpoint)
		if normalizeErr != nil {
			return nil, nil, normalizeErr
		}
		streamPayload["endpoint"] = normalizedEndpoint
		if headersRaw, exists := streamPayload["headers"]; exists {
			headers, headersErr := parseStringMap(headersRaw, "stream_http.headers")
			if headersErr != nil {
				return nil, nil, headersErr
			}
			streamPayload["headers"] = headers
		}
		payload["stream_http"] = streamPayload
	}

	normalized, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal config: %w", err)
	}
	return json.RawMessage(normalized), payload, nil
}

func buildProbeConfig(payload map[string]any) (toolruntime.MCPServerConfig, []string, error) {
	transport, ok := payload["transport"].(string)
	if !ok || strings.TrimSpace(transport) == "" {
		return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: transport is required", ErrInvalidArgument)
	}

	probeConfig := toolruntime.MCPServerConfig{
		ID:        "manager-validator",
		Transport: transport,
		TimeoutMs: defaultProbeTimeoutMs,
	}
	if timeoutRaw, exists := payload["timeout_ms"]; exists {
		timeout, ok := timeoutRaw.(int64)
		if !ok {
			return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: timeout_ms must be integer", ErrInvalidArgument)
		}
		probeConfig.TimeoutMs = int(timeout)
	}

	toolNameList := make([]string, 0)
	if toolNamesRaw, exists := payload["tool_name_list"]; exists {
		toolNames, ok := toolNamesRaw.([]string)
		if !ok {
			return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: tool_name_list must be string array", ErrInvalidArgument)
		}
		toolNameList = append(toolNameList, toolNames...)
		probeConfig.ToolNameList = append([]string(nil), toolNameList...)
	}

	authPayload := map[string]any{"type": "none"}
	if authRaw, exists := payload["auth"]; exists {
		parsedAuth, ok := authRaw.(map[string]any)
		if !ok {
			return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: auth must be json object", ErrInvalidArgument)
		}
		authPayload = parsedAuth
	}

	switch transport {
	case "stdio":
		stdioRaw, ok := payload["stdio"]
		if !ok {
			return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: stdio is required", ErrInvalidArgument)
		}
		stdioPayload, ok := stdioRaw.(map[string]any)
		if !ok {
			return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: stdio must be json object", ErrInvalidArgument)
		}

		command, err := requireStringField(stdioPayload, "command")
		if err != nil {
			return toolruntime.MCPServerConfig{}, nil, err
		}
		probeConfig.Command = strings.TrimSpace(command)

		if argsRaw, exists := stdioPayload["args"]; exists {
			args, ok := argsRaw.([]string)
			if !ok {
				return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: stdio.args must be string array", ErrInvalidArgument)
			}
			probeConfig.Args = append([]string(nil), args...)
		}
		if envRaw, exists := stdioPayload["env"]; exists {
			env, err := extractStringMap(envRaw, "stdio.env")
			if err != nil {
				return toolruntime.MCPServerConfig{}, nil, err
			}
			probeConfig.Env = env
		}
		if cwdRaw, exists := stdioPayload["cwd"]; exists {
			cwd, ok := cwdRaw.(string)
			if !ok {
				return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: stdio.cwd must be string", ErrInvalidArgument)
			}
			probeConfig.CWD = strings.TrimSpace(cwd)
		}
	case "sse":
		sseRaw, ok := payload["sse"]
		if !ok {
			return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: sse is required", ErrInvalidArgument)
		}
		ssePayload, ok := sseRaw.(map[string]any)
		if !ok {
			return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: sse must be json object", ErrInvalidArgument)
		}
		endpoint, err := requireStringField(ssePayload, "endpoint")
		if err != nil {
			return toolruntime.MCPServerConfig{}, nil, err
		}
		probeConfig.Endpoint = strings.TrimSpace(endpoint)

		headers := make(map[string]string)
		if headersRaw, exists := ssePayload["headers"]; exists {
			parsedHeaders, headersErr := extractStringMap(headersRaw, "sse.headers")
			if headersErr != nil {
				return toolruntime.MCPServerConfig{}, nil, headersErr
			}
			headers = parsedHeaders
		}
		probeConfig.Headers, err = applyAuthHeaders(headers, authPayload)
		if err != nil {
			return toolruntime.MCPServerConfig{}, nil, err
		}
	case "stream_http":
		streamRaw, ok := payload["stream_http"]
		if !ok {
			return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: stream_http is required", ErrInvalidArgument)
		}
		streamPayload, ok := streamRaw.(map[string]any)
		if !ok {
			return toolruntime.MCPServerConfig{}, nil, fmt.Errorf("%w: stream_http must be json object", ErrInvalidArgument)
		}
		endpoint, err := requireStringField(streamPayload, "endpoint")
		if err != nil {
			return toolruntime.MCPServerConfig{}, nil, err
		}
		probeConfig.Endpoint = strings.TrimSpace(endpoint)

		headers := make(map[string]string)
		if headersRaw, exists := streamPayload["headers"]; exists {
			parsedHeaders, headersErr := extractStringMap(headersRaw, "stream_http.headers")
			if headersErr != nil {
				return toolruntime.MCPServerConfig{}, nil, headersErr
			}
			headers = parsedHeaders
		}
		probeConfig.Headers, err = applyAuthHeaders(headers, authPayload)
		if err != nil {
			return toolruntime.MCPServerConfig{}, nil, err
		}
	}

	return probeConfig, toolNameList, nil
}

func probeMCPServer(ctx context.Context, cfg toolruntime.MCPServerConfig, requiredTools []string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultProbeTimeoutMs * time.Millisecond
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	loader, err := toolruntime.NewMCPLoader(cfg)
	if err != nil {
		return fmt.Errorf("create mcp loader: %w", err)
	}
	defer func() {
		_ = loader.Close()
	}()

	loadedTools, err := loader.Load(probeCtx)
	if err != nil {
		return fmt.Errorf("connect and list tools failed: %w", err)
	}

	if err := ensureRequiredTools(loadedTools, requiredTools, probeCtx); err != nil {
		return err
	}

	return nil
}

func ensureRequiredTools(loadedTools []einoTool.BaseTool, requiredTools []string, ctx context.Context) error {
	if len(requiredTools) == 0 {
		return nil
	}

	foundTools := make(map[string]struct{}, len(loadedTools))
	for _, loaded := range loadedTools {
		info, err := loaded.Info(ctx)
		if err != nil {
			continue
		}
		if _, toolName, ok := toolruntime.ParseMCPToolName(info.Name); ok {
			foundTools[toolName] = struct{}{}
			continue
		}
		foundTools[info.Name] = struct{}{}
	}

	missing := make([]string, 0)
	for _, required := range requiredTools {
		required = strings.TrimSpace(required)
		if required == "" {
			continue
		}
		if _, exists := foundTools[required]; !exists {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("%w: tool_name_list contains unavailable tools: %s", ErrInvalidArgument, strings.Join(missing, ","))
	}

	return nil
}

func applyAuthHeaders(headers map[string]string, authPayload map[string]any) (map[string]string, error) {
	mergedHeaders := make(map[string]string, len(headers)+1)
	for key, value := range headers {
		mergedHeaders[key] = value
	}

	authType := "none"
	if typeRaw, exists := authPayload["type"]; exists {
		parsedType, ok := typeRaw.(string)
		if !ok {
			return nil, fmt.Errorf("%w: auth.type must be string", ErrInvalidArgument)
		}
		authType = strings.ToLower(strings.TrimSpace(parsedType))
	}

	switch authType {
	case "none", "":
		return mergedHeaders, nil
	case "bearer", "api_key":
	default:
		return nil, fmt.Errorf("%w: auth.type %q is not supported", ErrInvalidArgument, authType)
	}

	tokenRaw, exists := authPayload["token"]
	if !exists {
		return nil, fmt.Errorf("%w: auth.token is required when auth.type is %s", ErrInvalidArgument, authType)
	}
	token, ok := tokenRaw.(string)
	if !ok {
		return nil, fmt.Errorf("%w: auth.token must be string", ErrInvalidArgument)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("%w: auth.token is required when auth.type is %s", ErrInvalidArgument, authType)
	}

	header := "Authorization"
	if headerRaw, exists := authPayload["header"]; exists {
		parsedHeader, ok := headerRaw.(string)
		if !ok {
			return nil, fmt.Errorf("%w: auth.header must be string", ErrInvalidArgument)
		}
		if strings.TrimSpace(parsedHeader) != "" {
			header = strings.TrimSpace(parsedHeader)
		}
	}

	if authType == "bearer" && !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	mergedHeaders[header] = token

	return mergedHeaders, nil
}

func extractStringMap(raw any, field string) (map[string]string, error) {
	switch value := raw.(type) {
	case map[string]string:
		cloned := make(map[string]string, len(value))
		for key, item := range value {
			cloned[key] = item
		}
		return cloned, nil
	case map[string]any:
		cloned := make(map[string]string, len(value))
		for key, item := range value {
			strItem, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%w: %s values must be string", ErrInvalidArgument, field)
			}
			cloned[key] = strItem
		}
		return cloned, nil
	default:
		return nil, fmt.Errorf("%w: %s must be json object", ErrInvalidArgument, field)
	}
}

func parseObject(raw any, field string) (map[string]any, error) {
	if raw == nil {
		return nil, fmt.Errorf("%w: %s is required", ErrInvalidArgument, field)
	}
	parsed, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s must be json object", ErrInvalidArgument, field)
	}
	return parsed, nil
}

func requireStringField(payload map[string]any, field string) (string, error) {
	raw, exists := payload[field]
	if !exists {
		return "", fmt.Errorf("%w: %s is required", ErrInvalidArgument, field)
	}
	value, err := parseOptionalString(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %s must be string", ErrInvalidArgument, field)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%w: %s is required", ErrInvalidArgument, field)
	}
	return value, nil
}

func parseOptionalString(raw any) (string, error) {
	value, ok := raw.(string)
	if !ok {
		return "", errors.New("not string")
	}
	return value, nil
}

func parsePositiveInteger(raw any) (int64, error) {
	value, ok := raw.(float64)
	if !ok {
		return 0, errors.New("not number")
	}
	if value <= 0 || value != float64(int64(value)) {
		return 0, errors.New("not positive integer")
	}
	return int64(value), nil
}

func parseStringArray(raw any) ([]string, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, errors.New("not array")
	}
	parsed := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, errors.New("array item not string")
		}
		parsed = append(parsed, strings.TrimSpace(value))
	}
	return parsed, nil
}

func parseStringMap(raw any, field string) (map[string]string, error) {
	payload, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s must be json object", ErrInvalidArgument, field)
	}
	parsed := make(map[string]string, len(payload))
	for key, value := range payload {
		strValue, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %s values must be string", ErrInvalidArgument, field)
		}
		parsed[key] = strValue
	}
	return parsed, nil
}

func normalizeEndpoint(raw string) (string, error) {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return "", fmt.Errorf("%w: endpoint is required", ErrInvalidArgument)
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("%w: endpoint %q is invalid", ErrInvalidArgument, raw)
	}
	return endpoint, nil
}
