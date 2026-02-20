package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/toolentitlement"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type fakeRuntimeEntitlementReader struct {
	entry toolentitlement.RepoEntry
	err   error
}

func (r *fakeRuntimeEntitlementReader) GetRepoEntry(_ context.Context, _ uuid.UUID, _ uuid.UUID) (toolentitlement.RepoEntry, error) {
	if r.err != nil {
		return toolentitlement.RepoEntry{}, r.err
	}
	return r.entry, nil
}

func TestService_ListToolsSuccess(t *testing.T) {
	endpoint, cleanup := startRuntimeStreamHTTPServer(t)
	defer cleanup()

	service := NewService(&fakeRuntimeEntitlementReader{entry: buildRuntimeRepoEntry(endpoint, true)})
	items, err := service.ListTools(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(items))
	}
	if items[0].Name != "get_device_status" {
		t.Fatalf("expected first tool get_device_status, got %s", items[0].Name)
	}
	if items[1].Name != "ping" {
		t.Fatalf("expected second tool ping, got %s", items[1].Name)
	}
}

func TestService_CallToolSuccess(t *testing.T) {
	endpoint, cleanup := startRuntimeStreamHTTPServer(t)
	defer cleanup()

	service := NewService(&fakeRuntimeEntitlementReader{entry: buildRuntimeRepoEntry(endpoint, true)})
	result, err := service.CallTool(context.Background(), uuid.New(), uuid.New(), "ping", nil)
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.ToolName != "ping" {
		t.Fatalf("expected tool_name ping, got %s", result.ToolName)
	}
	resultRaw, _ := json.Marshal(result.Result)
	if !strings.Contains(string(resultRaw), "pong") {
		t.Fatalf("expected result to contain pong, got %s", string(resultRaw))
	}
}

func TestService_CallToolRejectsMissingTool(t *testing.T) {
	endpoint, cleanup := startRuntimeStreamHTTPServer(t)
	defer cleanup()

	service := NewService(&fakeRuntimeEntitlementReader{entry: buildRuntimeRepoEntry(endpoint, true)})
	_, err := service.CallTool(context.Background(), uuid.New(), uuid.New(), "not_exists", map[string]any{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestService_ListToolsRejectsUnusableEntitlement(t *testing.T) {
	service := NewService(&fakeRuntimeEntitlementReader{entry: buildRuntimeRepoEntry("http://127.0.0.1:1/mcp", false)})
	_, err := service.ListTools(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrBusinessRule) {
		t.Fatalf("expected ErrBusinessRule, got %v", err)
	}
}

func TestService_ListToolsRejectsUnreachableEndpoint(t *testing.T) {
	service := NewService(&fakeRuntimeEntitlementReader{entry: buildRuntimeRepoEntry("http://127.0.0.1:1/mcp", true)})
	_, err := service.ListTools(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrBusinessRule) {
		t.Fatalf("expected ErrBusinessRule, got %v", err)
	}
}

func buildRuntimeRepoEntry(endpoint string, usable bool) toolentitlement.RepoEntry {
	itemID := uuid.New()
	entitlementID := uuid.New()

	return toolentitlement.RepoEntry{
		Entitlement: toolentitlement.Entitlement{
			ID:         entitlementID,
			UserID:     uuid.New(),
			ToolItemID: itemID,
			Status:     contracts.EntitlementStatusActive,
			StartsAt:   time.Now().UTC().Add(-time.Minute),
		},
		Item: toolmarket.Item{
			ID:       itemID,
			Protocol: contracts.ToolProtocolMCP,
			Status:   contracts.ToolStatusActive,
			Config: []byte(`{
				"transport": "stream_http",
				"timeout_ms": 3000,
				"stream_http": {"endpoint": "` + endpoint + `"}
			}`),
		},
		IsUsable: usable,
	}
}

func startRuntimeStreamHTTPServer(t *testing.T) (string, func()) {
	t.Helper()

	addr := allocateRuntimeTCPAddr(t)
	baseURL := "http://" + addr
	mcpServer := newRuntimeTestMCPServer()
	httpServer := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithEndpointPath("/mcp"),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Start(addr)
	}()

	waitRuntimeTCPReady(t, addr)

	return baseURL + "/mcp", func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				t.Logf("runtime stream_http server shutdown with error: %v", err)
			}
		case <-time.After(2 * time.Second):
		}
	}
}

func newRuntimeTestMCPServer() *server.MCPServer {
	mcpServer := server.NewMCPServer("runtime-test", "1.0.0")
	mcpServer.AddTool(
		mcp.NewTool("ping", mcp.WithDescription("ping tool")),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("pong"), nil
		},
	)
	mcpServer.AddTool(
		mcp.NewTool("get_device_status", mcp.WithDescription("get device status")),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	)
	return mcpServer
}

func allocateRuntimeTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func waitRuntimeTCPReady(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server at %s not ready in time: %v", addr, err)
		}
		time.Sleep(80 * time.Millisecond)
	}
}
