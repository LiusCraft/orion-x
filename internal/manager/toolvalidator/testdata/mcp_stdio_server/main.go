package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	mcpServer := server.NewMCPServer("validator-stdio", "1.0.0")
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

	if err := server.ServeStdio(mcpServer); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "serve stdio error: %v\n", err)
		os.Exit(1)
	}
}
