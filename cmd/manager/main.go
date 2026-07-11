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

	"golang.org/x/crypto/bcrypt"

	"github.com/liuscraft/orion-x/cmd/manager/handler"
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai"
	"github.com/liuscraft/orion-x/internal/logging"
	_ "github.com/liuscraft/orion-x/internal/provider/asr/register"
	_ "github.com/liuscraft/orion-x/internal/provider/tts/register"
	"github.com/liuscraft/orion-x/internal/store"
)

var timeNow = time.Now

func main() {
	configPath := flag.String("config", "data/manager.yaml", "config file path")
	flag.Parse()

	cfg, err := loadManagerConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(1)
	}

	if err := logging.Init(logging.Config{Level: cfg.Logging.Level, Format: cfg.Logging.Format}); err != nil {
		fmt.Fprintln(os.Stderr, "init logger:", err)
		os.Exit(1)
	}
	defer logging.Sync()

	if strings.TrimSpace(cfg.Database.DSN) == "" {
		logging.Fatalf("database.dsn is required")
	}
	if strings.TrimSpace(cfg.JWT.Secret) == "" {
		logging.Fatalf("jwt.secret is required")
	}

	db, err := store.Open(cfg.Database.DSN)
	if err != nil {
		logging.Fatalf("open db: %v", err)
	}

	users := store.NewUserStore(db)
	voicebots := store.NewVoicebotStore(db)
	devices := store.NewDeviceStore(db)
	providers := store.NewProviderStore(db)
	models := store.NewAIModelStore(db)
	languages := store.NewLanguageStore(db)
	voices := store.NewModelVoiceStore(db, languages)
	mcpMarket := store.NewMCPMarketStore(db)
	mcpServers := store.NewMCPServerStore(db)
	mcpBindings := store.NewVoicebotMCPBindingStore(db)

	if pass := strings.TrimSpace(cfg.Admin.Password); pass != "" {
		if _, err := users.GetByUsername(cfg.Admin.Username); errors.Is(err, store.ErrNotFound) {
			hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
			if err != nil {
				logging.Fatalf("hash admin password: %v", err)
			}
			if _, err := users.Create(cfg.Admin.Username, string(hash), "system"); err != nil {
				logging.Fatalf("create admin user: %v", err)
			}
			logging.Infof("created admin user %q", cfg.Admin.Username)
		}
	}

	secret := []byte(cfg.JWT.Secret)
	sign := func(userID string) (string, error) { return signToken(secret, userID) }

	langH := handler.NewLanguageHandler(languages)

	r := newRouter(secret, users, voicebots, devices, providers, models, voices, langH, mcpMarket, mcpServers, mcpBindings, sign)
	srv := &http.Server{Addr: cfg.Server.Addr, Handler: r}

	go func() {
		logging.Infof("manager listening on %s", cfg.Server.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.Fatalf("listen: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logging.Errorf("shutdown: %v", err)
	}
	logging.Infof("manager stopped")
}
