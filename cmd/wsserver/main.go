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
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	_ "github.com/liuscraft/orion-x/internal/provider/asr/register"
	_ "github.com/liuscraft/orion-x/internal/provider/tts/register"
	"github.com/liuscraft/orion-x/internal/tools"
)

const shutdownTimeout = 10 * time.Second

func main() {
	configPath := flag.String("config", defaultWsserverConfigPath, "config file path")
	flag.Parse()

	cfg, err := loadWsserverConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := logging.Init(logging.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logging.Sync()

	managerURL := strings.TrimSpace(cfg.Manager.URL)
	if managerURL == "" {
		logging.Fatalf("manager.url is required — all device configs are loaded from manager")
	}

	logging.SetTraceID(logging.NewTraceID())
	logging.Infof("========================================")
	logging.Infof("        WS VoiceBot Server Starting... ")
	logging.Infof("========================================")

	baseCtx := context.Background()

	// process-level memory / tool / agent — shared across connections.
	// Per-connection memory service is created in newConnection.
	memorySvc, err := memory.NewService(memory.Config{}, memory.Options{
		SystemPrompt: agent.DefaultSystemPrompt(),
		ManagerURL:   managerURL,
		DeviceID:     "",
		ReviewConfig: memory.ReviewConfig{Enabled: false},
	})
	if err != nil {
		logging.Fatalf("Failed to create memory service: %v", err)
	}
	defer func() {
		if err := memorySvc.Close(); err != nil {
			logging.Warnf("Close memory service failed: %v", err)
		}
	}()

	toolMgr, err := tools.NewManager(baseCtx, tools.ManagerConfig{})
	if err != nil {
		logging.Fatalf("Failed to create ToolManager: %v", err)
	}
	defer func() {
		if err := toolMgr.Close(); err != nil {
			logging.Warnf("Close ToolManager failed: %v", err)
		}
	}()

	deviceCfg := newHTTPDeviceConfigLoader(managerURL)
	logging.Infof("Device config loader: manager at %s", managerURL)

	srv := NewServer(toolMgr, memorySvc, deviceCfg)

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Server.WsPath, srv.HandleWS)
	httpServer := &http.Server{Addr: cfg.Server.Addr, Handler: mux}

	go func() {
		logging.Infof("Listening on %s (ws: %s)", cfg.Server.Addr, cfg.Server.WsPath)
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logging.Errorf("HTTP server shutdown error: %v", err)
	}
	srv.Shutdown(shutdownTimeout)

	logging.Infof("WS VoiceBot Server stopped.")
	logging.Sync()

	_, _, _ = syscall.Syscall(syscall.SYS_EXIT, 0, 0, 0)
}
