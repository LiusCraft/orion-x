// Command wsserver is the unified entry point for Orion-X server connectors.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	_ "github.com/liuscraft/orion-x/internal/provider/asr/register"
	_ "github.com/liuscraft/orion-x/internal/provider/tts/register"
	"github.com/liuscraft/orion-x/internal/tools"

	"github.com/liuscraft/orion-x/internal/connector"
	"github.com/liuscraft/orion-x/internal/connector/tg"
	"github.com/liuscraft/orion-x/internal/connector/xiaozhi"
)

func main() {
	configPath := flag.String("config", "data/wsserver.yaml", "config file path")
	flag.Parse()

	cfg, err := xiaozhi.LoadConfig(*configPath)
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

	// Shared dependencies for connectors
	deviceCfgLoader := xiaozhi.NewHTTPDeviceConfigLoader(cfg.Manager.URL)
	deps := &connector.Dependencies{
		DeviceCfgLoader: deviceCfgLoader,
	}

	connMgr := connector.NewManager(deps)

	// Xiaozhi WS Connector
	wsConn := xiaozhi.NewXiaozhiWSConnector(cfg, deps, toolMgr, memorySvc)
	connMgr.Register(wsConn)

	// TG Bot Connector — started automatically; refreshes device list
	// from the manager and starts bot instances for devices with tokens.
	tgConn := tg.NewTGConnector(deps, toolMgr, memorySvc)
	connMgr.Register(tgConn)

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	if err := connMgr.Start(ctx); err != nil {
		logging.Fatalf("Failed to start connectors: %v", err)
	}

	logging.Infof("========================================")
	logging.Infof("     Orion-X Server is Running!        ")
	logging.Infof("     Press Ctrl+C to stop.             ")
	logging.Infof("========================================")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logging.Infof("Shutting down...")
	cancel()
	connMgr.Stop()

	logging.Infof("Orion-X Server stopped.")
	logging.Sync()

	_, _, _ = syscall.Syscall(syscall.SYS_EXIT, 0, 0, 0)
}
