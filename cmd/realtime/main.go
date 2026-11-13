// Command realtime serves the OpenAI Realtime GA WebSocket endpoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	apikeyclient "github.com/liuscraft/orion-x/internal/apikey/client"
	"github.com/liuscraft/orion-x/internal/channels"
	realtimechannel "github.com/liuscraft/orion-x/internal/channels/realtime"
	"github.com/liuscraft/orion-x/internal/channels/xiaozhi"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/provider"
	_ "github.com/liuscraft/orion-x/internal/provider/asr/register"
	_ "github.com/liuscraft/orion-x/internal/provider/llm/provider/anthropic/messages"
	_ "github.com/liuscraft/orion-x/internal/provider/llm/provider/openai"
	_ "github.com/liuscraft/orion-x/internal/provider/llm/provider/openai/responses"
	_ "github.com/liuscraft/orion-x/internal/provider/tts/register"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/task"
	"github.com/liuscraft/orion-x/internal/tools"
)

func main() {
	configPath := flag.String("config", "data/wsserver.yaml", "config file path")
	addr := flag.String("addr", ":8090", "Realtime WebSocket listen address")
	flag.Parse()

	serverCfg, err := xiaozhi.LoadConfig(*configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}
	if err := xiaozhi.ValidateManagerURL(serverCfg.Manager.URL); err != nil {
		fatalf("invalid manager configuration: %v", err)
	}
	serverCfg.Manager.URL = strings.TrimRight(strings.TrimSpace(serverCfg.Manager.URL), "/")
	if err := logging.Init(logging.Config{Level: serverCfg.Logging.Level, Format: serverCfg.Logging.Format}); err != nil {
		fatalf("init logger: %v", err)
	}
	defer logging.Sync()

	baseCtx := context.Background()
	memorySvc, err := memory.NewService(memory.Config{}, memory.Options{
		ManagerURL:   serverCfg.Manager.URL,
		ReviewConfig: memory.ReviewConfig{Enabled: false},
	})
	if err != nil {
		fatalf("create memory service: %v", err)
	}
	defer func() { _ = memorySvc.Close() }()

	toolMgr, err := tools.NewManager(baseCtx, tools.ManagerConfig{})
	if err != nil {
		fatalf("create tool manager: %v", err)
	}
	defer func() { _ = toolMgr.Close() }()

	deviceCfgLoader := xiaozhi.NewHTTPDeviceConfigLoader(serverCfg.Manager.URL)
	var keyVerifier channels.KeyVerifier
	if serverCfg.Auth.AuthEnabled() {
		if err := xiaozhi.ValidateAuthConfig(serverCfg); err != nil {
			fatalf("invalid auth configuration: %v", err)
		}
		keyVerifier = apikeyclient.New(apikeyclient.Config{
			BaseURL: serverCfg.Manager.URL,
			Token:   serverCfg.Manager.Token,
			Timeout: serverCfg.Auth.Timeout,
		})
	}
	sessions := session.NewManager()
	deps := &channels.Dependencies{
		DeviceCfgLoader: deviceCfgLoader,
		Sessions:        sessions,
		Tasks:           task.NewRegistry(sessions),
		Providers:       provider.NewPool(),
		KeyVerifier:     keyVerifier,
	}
	ch := realtimechannel.NewChannel(&realtimechannel.Config{Addr: *addr}, deps, toolMgr, memorySvc)
	ctx, cancel := signal.NotifyContext(baseCtx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := ch.Start(ctx); err != nil {
		fatalf("start realtime channel: %v", err)
	}
	<-ctx.Done()
	_ = ch.Stop(context.Background())
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
