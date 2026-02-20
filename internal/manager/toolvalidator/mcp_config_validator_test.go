package toolvalidator

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestMCPConfigValidator_ValidateSSEConfigWithLiveProbe(t *testing.T) {
	endpoint, cleanup := startSSEServer(t)
	defer cleanup()

	validator := NewMCPConfigValidator()
	raw := mustMarshalJSON(t, map[string]any{
		"transport":      "sse",
		"timeout_ms":     5000,
		"tool_name_list": []string{"ping"},
		"sse": map[string]any{
			"endpoint": endpoint,
		},
	})

	normalized, err := validator.Validate(context.Background(), contracts.ToolProtocolMCP, raw)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(normalized, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["transport"] != "sse" {
		t.Fatalf("expected transport sse, got %#v", payload["transport"])
	}
}

func TestMCPConfigValidator_ValidateStreamHTTPConfigWithLiveProbe(t *testing.T) {
	endpoint, cleanup := startStreamHTTPServer(t)
	defer cleanup()

	validator := NewMCPConfigValidator()
	raw := mustMarshalJSON(t, map[string]any{
		"transport":      "stream_http",
		"timeout_ms":     5000,
		"tool_name_list": []string{"get_device_status"},
		"stream_http": map[string]any{
			"endpoint": endpoint,
		},
	})

	_, err := validator.Validate(context.Background(), contracts.ToolProtocolMCP, raw)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestMCPConfigValidator_ValidateStdioConfigWithLiveProbe(t *testing.T) {
	validator := NewMCPConfigValidator()
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	stdioServerPath := filepath.Join(workingDir, "testdata", "mcp_stdio_server")

	raw := mustMarshalJSON(t, map[string]any{
		"transport":      "stdio",
		"timeout_ms":     20000,
		"tool_name_list": []string{"ping"},
		"stdio": map[string]any{
			"command": "go",
			"args":    []string{"run", stdioServerPath},
			"cwd":     workingDir,
			"env": map[string]string{
				"GO111MODULE": "on",
			},
		},
	})

	_, err = validator.Validate(context.Background(), contracts.ToolProtocolMCP, raw)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestMCPConfigValidator_RejectsMissingToolFromToolNameList(t *testing.T) {
	endpoint, cleanup := startSSEServer(t)
	defer cleanup()

	validator := NewMCPConfigValidator()
	raw := mustMarshalJSON(t, map[string]any{
		"transport":      "sse",
		"timeout_ms":     5000,
		"tool_name_list": []string{"not_exist_tool"},
		"sse": map[string]any{
			"endpoint": endpoint,
		},
	})

	_, err := validator.Validate(context.Background(), contracts.ToolProtocolMCP, raw)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
	if !strings.Contains(err.Error(), "tool_name_list") {
		t.Fatalf("expected error to mention tool_name_list, got %v", err)
	}
}

func TestMCPConfigValidator_RejectsUnreachableEndpoint(t *testing.T) {
	validator := NewMCPConfigValidator()
	raw := mustMarshalJSON(t, map[string]any{
		"transport":  "stream_http",
		"timeout_ms": 1500,
		"stream_http": map[string]any{
			"endpoint": "http://127.0.0.1:1/mcp",
		},
	})

	_, err := validator.Validate(context.Background(), contracts.ToolProtocolMCP, raw)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func mustMarshalJSON(t *testing.T, payload map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return json.RawMessage(raw)
}

func startSSEServer(t *testing.T) (string, func()) {
	t.Helper()

	addr := allocateTCPAddr(t)
	baseURL := "http://" + addr
	mcpServer := newValidatorTestMCPServer()
	sseServer := server.NewSSEServer(
		mcpServer,
		server.WithBaseURL(baseURL),
		server.WithSSEEndpoint("/sse"),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- sseServer.Start(addr)
	}()

	waitTCPReady(t, addr)

	return baseURL + "/sse", func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = sseServer.Shutdown(shutdownCtx)
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				t.Logf("sse server shutdown with error: %v", err)
			}
		case <-time.After(2 * time.Second):
		}
	}
}

func startStreamHTTPServer(t *testing.T) (string, func()) {
	t.Helper()

	addr := allocateTCPAddr(t)
	baseURL := "http://" + addr
	mcpServer := newValidatorTestMCPServer()
	httpServer := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithEndpointPath("/mcp"),
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.Start(addr)
	}()

	waitTCPReady(t, addr)

	return baseURL + "/mcp", func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				t.Logf("stream_http server shutdown with error: %v", err)
			}
		case <-time.After(2 * time.Second):
		}
	}
}

func newValidatorTestMCPServer() *server.MCPServer {
	mcpServer := server.NewMCPServer("validator-test", "1.0.0")
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

func allocateTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func waitTCPReady(t *testing.T, addr string) {
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
