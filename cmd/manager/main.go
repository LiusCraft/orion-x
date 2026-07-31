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

	"github.com/liuscraft/orion-x/internal/knowledge"
	"github.com/liuscraft/orion-x/internal/knowledge/retriever"
	_ "github.com/liuscraft/orion-x/internal/llm/provider/anthropic/messages"
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai"
	_ "github.com/liuscraft/orion-x/internal/llm/provider/openai/responses"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/oauth"
	githuboauth "github.com/liuscraft/orion-x/internal/oauth/github"
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

	// 同步代码中注册的 system providers/models/voices 到数据库。
	// 使用 meta_hash 做增量对比，只更新有变化的记录。
	if err := store.SyncSystemProviders(db); err != nil {
		logging.Fatalf("sync system providers: %v", err)
	}

	users := store.NewUserStore(db)
	bindings := store.NewOAuthBindingStore(db)
	voicebots := store.NewVoicebotStore(db)
	devices := store.NewDeviceStore(db)
	providers := store.NewProviderStore(db)
	models := store.NewAIModelStore(db)
	voices := store.NewModelVoiceStore(db)
	mcpMarket := store.NewMCPMarketStore(db)
	mcpServers := store.NewMCPServerStore(db)
	mcpBindings := store.NewVoicebotMCPBindingStore(db)
	memStore := store.NewMemoryEntryStore(db)
	turnStore := store.NewTurnStore(db)
	kbStore := store.NewKnowledgeBaseStore(db)
	docStore := store.NewDocumentStore(db)
	voicebotKBs := store.NewVoicebotKBStore(db)
	// Try loading admin by username first (legacy), then fall through to create
	admin, adminErr := users.GetByUsername(cfg.Admin.Username)
	if errors.Is(adminErr, store.ErrNotFound) {
		if pass := strings.TrimSpace(cfg.Admin.Password); pass != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
			if err != nil {
				logging.Fatalf("hash admin password: %v", err)
			}
			// Admin uses username@admin.local as the email placeholder
			adminEmail := cfg.Admin.Username + "@admin.local"
			admin, adminErr = users.Create(adminEmail, cfg.Admin.Username, string(hash), "system")
			if adminErr != nil {
				logging.Fatalf("create admin user: %v", adminErr)
			}
			logging.Infof("created admin user %q (%s)", cfg.Admin.Username, adminEmail)
		}
	} else if adminErr != nil {
		logging.Fatalf("load admin user: %v", adminErr)
	}
	if admin != nil && !admin.IsAdmin {
		if err := users.SetAdmin(admin.ID, true); err != nil {
			logging.Fatalf("promote admin user: %v", err)
		}
	}

	// 迁移历史 users.github_id → oauth_bindings（一次性，幂等）
	if err := migrateGithubBindings(users, bindings); err != nil {
		logging.Fatalf("migrate github bindings: %v", err)
	}

	secret := []byte(cfg.JWT.Secret)
	sign := func(userID string, isAdmin bool) (string, error) { return signToken(secret, userID, isAdmin) }

	// Knowledge base service — always created, users must configure embedding model to use it.
	kbRet, err := retriever.NewPGVector(db, 1536)
	if err != nil {
		logging.Warnf("knowledge retriever init failed: %v", err)
	}
	var kbSvc *knowledge.Service
	if kbRet != nil {
		kbSvc = knowledge.NewService(kbStore, docStore, models, kbRet)
		logging.Infof("knowledge service ready")
	}

	// 注册第三方 OAuth 平台（仅配置完整时注册）
	if cfg.GithubOAuth.ClientID != "" && cfg.GithubOAuth.ClientSecret != "" {
		oauth.Register(githuboauth.New(cfg.GithubOAuth.ClientID, cfg.GithubOAuth.ClientSecret, cfg.GithubOAuth.RedirectURL))
		logging.Infof("oauth: registered github provider")
	}

	r := newRouter(secret, users, bindings, voicebots, devices, providers, models, voices, mcpMarket, mcpServers, mcpBindings, sign, memStore, turnStore, kbSvc, kbStore, docStore, voicebotKBs)
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

// migrateGithubBindings 将历史 users.github_id 迁移到 oauth_bindings 表。
// 幂等：已存在同 (user_id, provider) 绑定则覆盖，重复执行无副作用。
func migrateGithubBindings(users *store.UserStore, bindings *store.OAuthBindingStore) error {
	legacy, err := users.ListWithGithubID()
	if err != nil {
		return err
	}
	for _, u := range legacy {
		if err := bindings.Bind(u.ID, "github", u.GithubID, "migration"); err != nil {
			return fmt.Errorf("migrate github binding for user %s: %w", u.ID, err)
		}
	}
	if len(legacy) > 0 {
		logging.Infof("oauth: migrated %d legacy github bindings to oauth_bindings", len(legacy))
	}
	return nil
}
