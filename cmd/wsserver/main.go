package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/config"
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	_ "github.com/liuscraft/orion-x/internal/provider/asr/register"
	_ "github.com/liuscraft/orion-x/internal/provider/tts/register"
	"github.com/liuscraft/orion-x/internal/tools"
)

// shutdownTimeout bounds how long in-flight connections get to wind down
// once a shutdown signal is received.
const shutdownTimeout = 10 * time.Second

func main() {
	configPath := flag.String("config", config.DefaultPath, "config file path")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	wsPath := flag.String("path", "/ws", "WebSocket endpoint path")
	flag.Parse()

	appConfig, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	if err := appConfig.ValidateKeys(true, true, true); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid config: %v\n", err)
		os.Exit(1)
	}
	llmCfg := appConfig.Provider.LLM.OpenAI

	if err := logging.Init(logging.Config{
		Level:  appConfig.Logging.Level,
		Format: appConfig.Logging.Format,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logging.Sync()

	logging.SetTraceID(logging.NewTraceID())
	logging.Infof("========================================")
	logging.Infof("        WS VoiceBot Server Starting... ")
	logging.Infof("========================================")
	logging.Infof("Config loaded successfully")

	baseCtx := context.Background()

	// --- process-level shared resources: Memory / ToolManager / Agent ---
	// These are safe to share across concurrent connections (see Server's
	// doc comment). Each connection still gets its own Session, so
	// per-connection state never touches these.
	memCfg := memory.Config{
		Mode:                 memory.Mode(strings.TrimSpace(appConfig.Memory.Mode)),
		SessionMaxTurns:      appConfig.Memory.SessionMaxTurns,
		SessionSummaryEveryN: appConfig.Memory.SessionSummaryEveryN,
		LongTermDBPath:       appConfig.Memory.LongTermDBPath,
		LongTermMaxResults:   appConfig.Memory.LongTermMaxResults,
		RetentionDays:        appConfig.Memory.RetentionDays,
		FTSMinScore:          appConfig.Memory.FTSMinScore,
	}
	memorySvc, err := memory.NewService(memCfg, memory.Options{
		SystemPrompt: agent.DefaultSystemPrompt(),
		LLM: memory.LLMConfig{
			Provider: appConfig.Provider.LLM.Type,
			APIKey:   llmCfg.APIKey,
			BaseURL:  llmCfg.BaseURL,
			Model:    llmCfg.Model,
		},
	})
	if err != nil {
		logging.Fatalf("Failed to create memory service: %v", err)
	}
	defer func() {
		if err := memorySvc.Close(); err != nil {
			logging.Warnf("Close memory service failed: %v", err)
		}
	}()

	agentCfg := agent.Config{
		Provider:    appConfig.Provider.LLM.Type,
		APIKey:      llmCfg.APIKey,
		BaseURL:     llmCfg.BaseURL,
		Model:       llmCfg.Model,
		ExtraFields: llmCfg.ExtraFields,
	}
	toolCfg := tools.ManagerConfig{
		MCPServers: toToolsMCPServers(appConfig.Tools.MCP),
	}

	logging.Infof("Creating ToolManager...")
	toolMgr, err := tools.NewManager(baseCtx, toolCfg)
	if err != nil {
		logging.Fatalf("Failed to create ToolManager: %v", err)
	}
	defer func() {
		if err := toolMgr.Close(); err != nil {
			logging.Warnf("Close ToolManager failed: %v", err)
		}
	}()
	logging.Infof("ToolManager created successfully")

	logging.Infof("Creating Agent...")
	agentInst, err := agent.New(baseCtx, agentCfg, toolMgr, memorySvc)
	if err != nil {
		logging.Fatalf("Failed to create Agent: %v", err)
	}
	logging.Infof("Agent created successfully")

	srv := NewServer(appConfig, agentInst)

	mux := http.NewServeMux()
	mux.HandleFunc(*wsPath, srv.HandleWS)
	httpServer := &http.Server{Addr: *addr, Handler: mux}

	go func() {
		logging.Infof("Listening on %s (ws endpoint: %s)", *addr, *wsPath)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Fatalf("HTTP server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logging.Infof("========================================")
	logging.Infof("     WS VoiceBot Server is Running!    ")
	logging.Infof("     Press Ctrl+C to stop.             ")
	logging.Infof("========================================")

	<-sigCh
	logging.Infof("========================================")
	logging.Infof("     Received interrupt signal...       ")
	logging.Infof("========================================")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logging.Errorf("HTTP server shutdown error: %v", err)
	}

	logging.Infof("Closing active connections...")
	srv.Shutdown(shutdownTimeout)

	logging.Infof("WS VoiceBot Server stopped.")
	logging.Sync()

	// Any connection that used auto mode loads the Silero VAD model, which
	// links ONNX Runtime. Returning from main() normally runs C++ static
	// destructors on macOS that crash with "mutex lock failed: Invalid
	// argument" (same root cause as the PortAudio crash cmd/voicebot works
	// around — see its main.go). Bypass that cleanup entirely.
	_, _, _ = syscall.Syscall(syscall.SYS_EXIT, 0, 0, 0)
}

func toToolsMCPServers(cfgs []config.MCPServerConfig) []tools.MCPServerConfig {
	servers := make([]tools.MCPServerConfig, 0, len(cfgs))
	for _, cfg := range cfgs {
		servers = append(servers, tools.MCPServerConfig{
			ID:           cfg.ID,
			Transport:    cfg.Transport,
			Command:      cfg.Command,
			Args:         cfg.Args,
			Endpoint:     cfg.Endpoint,
			ToolNameList: cfg.ToolNameList,
			TimeoutMs:    cfg.TimeoutMs,
		})
	}
	return servers
}
