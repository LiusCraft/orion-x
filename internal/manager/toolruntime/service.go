package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	einoTool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolentitlement"
	toolpkg "github.com/liuscraft/orion-x/internal/tools"
)

const defaultMCPTimeoutMs = 30000

type Service struct {
	entitlementReader EntitlementReader
}

func NewService(entitlementReader EntitlementReader) *Service {
	return &Service{entitlementReader: entitlementReader}
}

func (s *Service) ListTools(ctx context.Context, userID, entitlementID uuid.UUID) ([]ToolDescriptor, error) {
	loadedTools, closeLoader, err := s.loadMCPTools(ctx, userID, entitlementID)
	if err != nil {
		return nil, err
	}
	defer closeLoader()

	items := make([]ToolDescriptor, 0, len(loadedTools))
	for _, loaded := range loadedTools {
		info, infoErr := loaded.Info(ctx)
		if infoErr != nil {
			return nil, fmt.Errorf("read tool info: %w", infoErr)
		}

		name := info.Name
		if _, shortName, ok := toolpkg.ParseMCPToolName(info.Name); ok {
			name = shortName
		}

		inputSchema, schemaErr := toJSONSchemaPayload(info)
		if schemaErr != nil {
			return nil, fmt.Errorf("marshal tool schema for %s: %w", name, schemaErr)
		}

		items = append(items, ToolDescriptor{
			Name:        name,
			Description: info.Desc,
			InputSchema: inputSchema,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	return items, nil
}

func (s *Service) CallTool(ctx context.Context, userID, entitlementID uuid.UUID, toolName string, arguments map[string]any) (ToolCallResult, error) {
	requestedName := strings.TrimSpace(toolName)
	if requestedName == "" {
		return ToolCallResult{}, fmt.Errorf("%w: tool_name is required", ErrInvalidArgument)
	}

	loadedTools, closeLoader, err := s.loadMCPTools(ctx, userID, entitlementID)
	if err != nil {
		return ToolCallResult{}, err
	}
	defer closeLoader()

	invokable, resolvedName, err := resolveInvokableTool(ctx, loadedTools, requestedName)
	if err != nil {
		return ToolCallResult{}, err
	}

	if arguments == nil {
		arguments = map[string]any{}
	}
	argsJSON, err := json.Marshal(arguments)
	if err != nil {
		return ToolCallResult{}, fmt.Errorf("%w: arguments must be valid json object", ErrInvalidArgument)
	}

	rawResult, err := invokable.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		return ToolCallResult{}, fmt.Errorf("%w: call tool %s failed: %s", ErrBusinessRule, resolvedName, err.Error())
	}

	return ToolCallResult{
		ToolName: resolvedName,
		Result:   parseToolOutput(rawResult),
	}, nil
}

func (s *Service) loadMCPTools(ctx context.Context, userID, entitlementID uuid.UUID) ([]einoTool.BaseTool, func(), error) {
	entry, err := s.loadUsableRepoEntry(ctx, userID, entitlementID)
	if err != nil {
		return nil, nil, err
	}

	mcpConfig, err := buildMCPServerConfig(entry, entitlementID)
	if err != nil {
		return nil, nil, err
	}

	loader, err := toolpkg.NewMCPLoader(mcpConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: create mcp loader failed: %s", ErrBusinessRule, err.Error())
	}

	loadTimeout := time.Duration(mcpConfig.TimeoutMs) * time.Millisecond
	if loadTimeout <= 0 {
		loadTimeout = defaultMCPTimeoutMs * time.Millisecond
	}
	loadCtx, cancelLoad := context.WithTimeout(ctx, loadTimeout)

	loadedTools, err := loader.Load(loadCtx)
	if err != nil {
		cancelLoad()
		_ = loader.Close()
		return nil, nil, fmt.Errorf("%w: connect and list tools failed: %s", ErrBusinessRule, err.Error())
	}

	cleanup := func() {
		cancelLoad()
		_ = loader.Close()
	}

	return loadedTools, cleanup, nil
}

func (s *Service) loadUsableRepoEntry(ctx context.Context, userID, entitlementID uuid.UUID) (toolentitlement.RepoEntry, error) {
	if s == nil || s.entitlementReader == nil {
		return toolentitlement.RepoEntry{}, errors.New("tool runtime service dependencies are not initialized")
	}
	if userID == uuid.Nil || entitlementID == uuid.Nil {
		return toolentitlement.RepoEntry{}, fmt.Errorf("%w: user_id and entitlement_id are required", ErrInvalidArgument)
	}

	entry, err := s.entitlementReader.GetRepoEntry(ctx, userID, entitlementID)
	if err != nil {
		switch {
		case errors.Is(err, toolentitlement.ErrInvalidArgument):
			return toolentitlement.RepoEntry{}, fmt.Errorf("%w: %s", ErrInvalidArgument, err.Error())
		case errors.Is(err, toolentitlement.ErrNotFound):
			return toolentitlement.RepoEntry{}, fmt.Errorf("%w: entitlement not found", ErrNotFound)
		case errors.Is(err, toolentitlement.ErrBusinessRule):
			return toolentitlement.RepoEntry{}, fmt.Errorf("%w: %s", ErrBusinessRule, err.Error())
		default:
			return toolentitlement.RepoEntry{}, fmt.Errorf("load entitlement repo entry: %w", err)
		}
	}

	if !entry.IsUsable {
		return toolentitlement.RepoEntry{}, fmt.Errorf("%w: entitlement is not usable", ErrBusinessRule)
	}
	if entry.Item.Status != contracts.ToolStatusActive {
		return toolentitlement.RepoEntry{}, fmt.Errorf("%w: tool market item is not active", ErrBusinessRule)
	}
	if entry.Item.Protocol != contracts.ToolProtocolMCP {
		return toolentitlement.RepoEntry{}, fmt.Errorf("%w: protocol %q is not supported", ErrBusinessRule, entry.Item.Protocol)
	}

	return entry, nil
}

func toJSONSchemaPayload(info *schema.ToolInfo) (any, error) {
	if info == nil || info.ParamsOneOf == nil {
		return nil, nil
	}
	jsonSchema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		return nil, err
	}
	if jsonSchema == nil {
		return nil, nil
	}

	raw, err := json.Marshal(jsonSchema)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func resolveInvokableTool(ctx context.Context, loadedTools []einoTool.BaseTool, requestedName string) (einoTool.InvokableTool, string, error) {
	requestedName = strings.TrimSpace(requestedName)
	if requestedName == "" {
		return nil, "", fmt.Errorf("%w: tool_name is required", ErrInvalidArgument)
	}

	var shortMatch einoTool.InvokableTool
	var shortMatchName string
	fullMatches := 0
	shortMatches := 0

	for _, loaded := range loadedTools {
		invokable, ok := loaded.(einoTool.InvokableTool)
		if !ok {
			continue
		}
		info, err := loaded.Info(ctx)
		if err != nil {
			continue
		}

		fullName := info.Name
		shortName := fullName
		if _, parsedShort, parsed := toolpkg.ParseMCPToolName(fullName); parsed {
			shortName = parsedShort
		}

		if fullName == requestedName {
			fullMatches++
			shortMatch = invokable
			shortMatchName = shortName
			continue
		}
		if shortName == requestedName {
			shortMatches++
			if fullMatches == 0 {
				shortMatch = invokable
				shortMatchName = shortName
			}
		}
	}

	if fullMatches > 1 || (fullMatches == 0 && shortMatches > 1) {
		return nil, "", fmt.Errorf("%w: tool_name %q is ambiguous", ErrInvalidArgument, requestedName)
	}
	if shortMatch == nil {
		return nil, "", fmt.Errorf("%w: tool_name %q not found", ErrInvalidArgument, requestedName)
	}

	return shortMatch, shortMatchName, nil
}

func parseToolOutput(raw string) any {
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}
	return payload
}

type mcpConfigPayload struct {
	Transport    string    `json:"transport"`
	TimeoutMs    int       `json:"timeout_ms"`
	ToolNameList []string  `json:"tool_name_list"`
	Auth         authInfo  `json:"auth"`
	Stdio        stdioInfo `json:"stdio"`
	SSE          httpInfo  `json:"sse"`
	StreamHTTP   httpInfo  `json:"stream_http"`
}

type authInfo struct {
	Type   string `json:"type"`
	Token  string `json:"token"`
	Header string `json:"header"`
}

type stdioInfo struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	CWD     string            `json:"cwd"`
}

type httpInfo struct {
	Endpoint string            `json:"endpoint"`
	Headers  map[string]string `json:"headers"`
}

func buildMCPServerConfig(entry toolentitlement.RepoEntry, entitlementID uuid.UUID) (toolpkg.MCPServerConfig, error) {
	rawConfig := entry.Item.Config
	if len(strings.TrimSpace(string(rawConfig))) == 0 {
		return toolpkg.MCPServerConfig{}, fmt.Errorf("%w: mcp config is required", ErrInvalidArgument)
	}

	var payload mcpConfigPayload
	if err := json.Unmarshal(rawConfig, &payload); err != nil {
		return toolpkg.MCPServerConfig{}, fmt.Errorf("%w: mcp config must be valid json", ErrInvalidArgument)
	}

	transport := strings.ToLower(strings.TrimSpace(payload.Transport))
	if transport == "streamable" {
		transport = "stream_http"
	}

	config := toolpkg.MCPServerConfig{
		ID:           "runtime-" + entitlementID.String(),
		Transport:    transport,
		ToolNameList: cloneStringSlice(payload.ToolNameList),
		TimeoutMs:    payload.TimeoutMs,
	}
	if config.TimeoutMs <= 0 {
		config.TimeoutMs = defaultMCPTimeoutMs
	}

	switch transport {
	case "stdio":
		command := strings.TrimSpace(payload.Stdio.Command)
		if command == "" {
			return toolpkg.MCPServerConfig{}, fmt.Errorf("%w: stdio.command is required", ErrInvalidArgument)
		}
		config.Command = command
		config.Args = cloneStringSlice(payload.Stdio.Args)
		config.CWD = strings.TrimSpace(payload.Stdio.CWD)
		config.Env = cloneStringMap(payload.Stdio.Env)
	case "sse":
		endpoint := strings.TrimSpace(payload.SSE.Endpoint)
		if endpoint == "" {
			return toolpkg.MCPServerConfig{}, fmt.Errorf("%w: sse.endpoint is required", ErrInvalidArgument)
		}
		headers, err := applyAuthHeaders(cloneStringMap(payload.SSE.Headers), payload.Auth)
		if err != nil {
			return toolpkg.MCPServerConfig{}, err
		}
		config.Endpoint = endpoint
		config.Headers = headers
	case "stream_http":
		endpoint := strings.TrimSpace(payload.StreamHTTP.Endpoint)
		if endpoint == "" {
			return toolpkg.MCPServerConfig{}, fmt.Errorf("%w: stream_http.endpoint is required", ErrInvalidArgument)
		}
		headers, err := applyAuthHeaders(cloneStringMap(payload.StreamHTTP.Headers), payload.Auth)
		if err != nil {
			return toolpkg.MCPServerConfig{}, err
		}
		config.Endpoint = endpoint
		config.Headers = headers
	default:
		return toolpkg.MCPServerConfig{}, fmt.Errorf("%w: unsupported transport %q", ErrInvalidArgument, payload.Transport)
	}

	return config, nil
}

func applyAuthHeaders(headers map[string]string, auth authInfo) (map[string]string, error) {
	merged := cloneStringMap(headers)
	authType := strings.ToLower(strings.TrimSpace(auth.Type))
	if authType == "" {
		authType = "none"
	}

	switch authType {
	case "none":
		return merged, nil
	case "bearer", "api_key":
	default:
		return nil, fmt.Errorf("%w: auth.type %q is not supported", ErrInvalidArgument, auth.Type)
	}

	token := strings.TrimSpace(auth.Token)
	if token == "" {
		return nil, fmt.Errorf("%w: auth.token is required when auth.type is %s", ErrInvalidArgument, authType)
	}

	header := strings.TrimSpace(auth.Header)
	if header == "" {
		header = "Authorization"
	}

	if authType == "bearer" && !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = "Bearer " + token
	}
	merged[header] = token

	return merged, nil
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		cloned = append(cloned, trimmed)
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
