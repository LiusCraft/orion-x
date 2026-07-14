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
	"github.com/liuscraft/orion-x/internal/knowledge"
	"github.com/liuscraft/orion-x/internal/knowledge/embedder"
	"github.com/liuscraft/orion-x/internal/knowledge/retriever"
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
	memStore := store.NewMemoryEntryStore(db)
	turnStore := store.NewTurnStore(db)
	kbStore := store.NewKnowledgeBaseStore(db)
	docStore := store.NewDocumentStore(db)

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

	// Knowledge base service — embedding API key from env, falls back to LLM key
	embCfg := embedder.Config{
		Type:   "openai",
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  "text-embedding-3-small",
	}
	emb, err := embedder.New(embCfg)
	if err != nil {
		logging.Warnf("knowledge embedder init failed, knowledge search disabled: %v", err)
	}
	var kbSvc *knowledge.Service
	if emb != nil {
		ret, err := retriever.NewPGVector(db, emb.Dimensions())
		if err != nil {
			logging.Warnf("knowledge retriever init failed: %v", err)
		} else {
			kbSvc = knowledge.NewService(kbStore, docStore, ret, emb)
			logging.Infof("knowledge service ready (dim=%d, model=%s)", emb.Dimensions(), embCfg.Model)
		}
	}

	r := newRouter(secret, users, voicebots, devices, providers, models, voices, langH, mcpMarket, mcpServers, mcpBindings, sign, memStore, turnStore, kbSvc, kbStore, docStore)
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
