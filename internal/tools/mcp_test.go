package tools

import (
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBuildMCPTransport_StreamableUsesExpandedHeaders(t *testing.T) {
	t.Setenv("MCP_TEST_TOKEN", "secret-token")

	transport, err := buildMCPTransport(MCPServerConfig{
		Transport: "streamable",
		Endpoint:  "https://example.com/mcp",
		Headers: map[string]string{
			"Authorization": "Bearer ${MCP_TEST_TOKEN}",
		},
	})
	if err != nil {
		t.Fatalf("buildMCPTransport failed: %v", err)
	}

	streamable, ok := transport.(*mcp.StreamableClientTransport)
	if !ok {
		t.Fatalf("expected streamable transport, got %T", transport)
	}
	if streamable.HTTPClient == nil {
		t.Fatal("expected custom HTTP client")
	}
	roundTripper, ok := streamable.HTTPClient.Transport.(headerRoundTripper)
	if !ok {
		t.Fatalf("expected headerRoundTripper, got %T", streamable.HTTPClient.Transport)
	}
	if got := roundTripper.headers["Authorization"]; got != "Bearer secret-token" {
		t.Fatalf("unexpected Authorization header: %q", got)
	}
}

func TestBuildMCPTransport_StdioUsesExpandedEnvAndCWD(t *testing.T) {
	t.Setenv("MCP_TEST_DIR", "/tmp/mcp-test-dir")

	transport, err := buildMCPTransport(MCPServerConfig{
		Transport: "stdio",
		Command:   "echo",
		Args:      []string{"ok"},
		Env:       map[string]string{"MCP_DIR": "${MCP_TEST_DIR}"},
		CWD:       "/tmp",
	})
	if err != nil {
		t.Fatalf("buildMCPTransport failed: %v", err)
	}

	command, ok := transport.(*mcp.CommandTransport)
	if !ok {
		t.Fatalf("expected command transport, got %T", transport)
	}
	if command.Command.Dir != "/tmp" {
		t.Fatalf("unexpected cwd: %q", command.Command.Dir)
	}
	if got := envValue(command.Command.Env, "MCP_DIR"); got != "/tmp/mcp-test-dir" {
		t.Fatalf("unexpected MCP_DIR: %q", got)
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if len(item) >= len(prefix) && item[:len(prefix)] == prefix {
			return item[len(prefix):]
		}
	}
	return os.Getenv(key)
}
