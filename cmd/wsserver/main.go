// Command wsserver is the unified entry point for Orion-X server connectors.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/liuscraft/orion-x/internal/llm/provider/anthropic/messages"
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai"
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai/responses"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/provider"
	_ "github.com/liuscraft/orion-x/internal/provider/asr/register"
	_ "github.com/liuscraft/orion-x/internal/provider/tts/register"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/task"
	"github.com/liuscraft/orion-x/internal/tools"

	"github.com/liuscraft/orion-x/internal/channels"
	"github.com/liuscraft/orion-x/internal/channels/tg"
	"github.com/liuscraft/orion-x/internal/channels/xiaozhi"
)

func main() {
	configPath := flag.String("config", "data/wsserver.yaml", "config file path")
	flag.Parse()

	cfg, err := xiaozhi.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	if err := xiaozhi.ValidateManagerURL(cfg.Manager.URL); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid manager configuration: %v\n", err)
		os.Exit(1)
	}
	cfg.Manager.URL = strings.TrimRight(strings.TrimSpace(cfg.Manager.URL), "/")

	if err := logging.Init(logging.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logging.Sync()

	logging.SetTraceID(logging.NewTraceID())
	logging.Infof("========================================")
	logging.Infof("        Orion-X Server Starting...      ")
	logging.Infof("========================================")

	baseCtx := context.Background()

	memorySvc, err := memory.NewService(memory.Config{}, memory.Options{
		ManagerURL:   cfg.Manager.URL,
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

	// Shared dependencies for channels
	deviceCfgLoader := xiaozhi.NewHTTPDeviceConfigLoader(cfg.Manager.URL)
	sessions := session.NewManager()
	deps := &channels.Dependencies{
		DeviceCfgLoader: deviceCfgLoader,
		Sessions:        sessions,
		Tasks:           task.NewRegistry(sessions),
		Providers:       provider.NewPool(),
	}

	chMgr := channels.NewManager(deps)

	// Xiaozhi WS Channel
	wsCh := xiaozhi.NewXiaozhiWSChannel(cfg, deps, toolMgr, memorySvc)
	chMgr.Register(wsCh)

	// TG Bot Channel — started automatically; refreshes device list
	// from the manager and starts bot instances for devices with tokens.
	tgCh := tg.NewTGChannel(deps, toolMgr, memorySvc)
	chMgr.Register(tgCh)

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	if err := chMgr.Start(ctx); err != nil {
		logging.Fatalf("Failed to start channels: %v", err)
	}
	health := newHealthServer(cfg.Health.Addr)
	if err := health.Start(); err != nil {
		logging.Fatalf("Failed to start health server: %v", err)
	}

	logging.Infof("========================================")
	logging.Infof("     Orion-X Server is Running!        ")
	logging.Infof("     Press Ctrl+C to stop.             ")
	logging.Infof("========================================")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logging.Infof("Shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := health.Stop(shutdownCtx); err != nil {
		logging.Warnf("Health server shutdown failed: %v", err)
	}
	shutdownCancel()
	cancel()
	chMgr.Stop()

	logging.Infof("Orion-X Server stopped.")
	logging.Sync()

	_, _, _ = syscall.Syscall(syscall.SYS_EXIT, 0, 0, 0)
}
