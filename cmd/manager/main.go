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
	"sync"
	"syscall"
	"time"
	_ "time/tzdata" // 把时区库编进二进制：billing.period_timezone 在没装 tzdata 的容器里也能解析

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/liuscraft/orion-x/internal/assets"
	"github.com/liuscraft/orion-x/internal/billing/service"
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
	"github.com/liuscraft/orion-x/internal/storage"
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

	// 同步系统智能体模板种子数据
	if err := store.SyncSystemTemplates(db); err != nil {
		logging.Warnf("sync system agent templates: %v", err)
	}

	// 同步内置计费项目录。billing_items 只是代码里那份常量的投影，失败也不该拦下
	// 进程（下次启动或管理员改一行 enabled 就好了）。
	if err := store.SyncBillingItems(db); err != nil {
		logging.Warnf("sync billing items: %v", err)
	}
	// 兜底价：只在某个会话计费项还没有任何生效价格时插入，不覆盖运营录的真实价格。
	// 没配 fallback_price_micro 而价格表又为空的话，会话会被 price_missing 拒掉——
	// 那时只能靠管理端录价格（§3.4）。
	if cfg.Billing.FallbackPriceMicro > 0 {
		created, err := store.SyncBillingFallbackPrices(db, cfg.Billing.FallbackPriceMicro)
		if err != nil {
			logging.Warnf("sync billing fallback prices: %v", err)
		} else if created > 0 {
			logging.Warnf("billing: seeded %d fallback price row(s) at %d micro/unit — 真实价格请用管理端 API 录（录完把 fallback_price_micro 清掉）",
				created, cfg.Billing.FallbackPriceMicro)
		}
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
	agentTemplates := store.NewAgentTemplateStore(db)
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

	// 对象存储 + 资源服务：未配置 storage 段时资源接口返回 503，manager 其余能力不受影响。
	assetSvc := initAssetService(cfg.Storage, db)

	// 注册第三方 OAuth 平台（仅配置完整时注册）
	if cfg.GithubOAuth.ClientID != "" && cfg.GithubOAuth.ClientSecret != "" {
		oauth.Register(githuboauth.New(cfg.GithubOAuth.ClientID, cfg.GithubOAuth.ClientSecret, cfg.GithubOAuth.RedirectURL))
		logging.Infof("oauth: registered github provider")
	}

	// 计费控制面。billing.enabled: false 时根本不建 service：所有 /api/billing/* 回
	// 503，也不跑结算 worker（§16.4 的 worker 就是 manager 进程里的 goroutine）。
	var billingSvc *service.Service
	if !cfg.Billing.Disabled() {
		billingCfg, err := cfg.Billing.ServiceConfig(timeNow)
		if err != nil {
			logging.Fatalf("billing config: %v", err)
		}
		billingSvc = service.New(
			store.NewBillingStore(db),
			service.NewSubjectResolver(devices, voicebots, models, voices),
			billingCfg,
		)
		if strings.TrimSpace(cfg.Internal.Token) == "" {
			logging.Warnf("billing: internal.token is not configured — /internal/billing/* will reject every request")
		}
		logging.Infof("billing ready: currency=%s reserve_seconds=%d settlement=%s overdraft=%s period_tz=%s",
			billingCfg.Currency, billingCfg.ReserveSeconds, billingCfg.SettlementMode,
			billingCfg.OverdraftPolicy, billingCfg.PeriodLocation)
	}

	// 结算 worker（tick 结算 + 预冻结回收）跟 HTTP 服务同进程，退出靠 cancel（§16.4）。
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	var workerWG sync.WaitGroup
	if billingSvc != nil {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			billingSvc.RunWorker(workerCtx)
		}()
	}

	r := newRouter(secret, users, bindings, voicebots, devices, providers, models, voices, mcpMarket, mcpServers, mcpBindings, sign, memStore, turnStore, kbSvc, kbStore, docStore, voicebotKBs, agentTemplates, assetSvc, cfg.Internal.Token, billingSvc)
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

	// §16.4：SIGTERM → cancel() → wg.Wait()，等在途的那批结算跑完。被截断的事务
	// 只会回滚，事件还是 pending，下次启动会重放，所以不会丢账。
	workerCancel()
	workerWG.Wait()
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

// initAssetService 构造对象存储与资源服务。
// 未配置或初始化失败时返回 nil，资源接口随后返回 503。
func initAssetService(cfg storage.Config, db *gorm.DB) *assets.Service {
	if !cfg.Enabled() {
		return nil
	}
	ctx := context.Background()
	backend, err := storage.New(ctx, cfg)
	if err != nil {
		logging.Errorf("object storage disabled: %v", err)
		return nil
	}
	if err := backend.Ping(ctx); err != nil {
		logging.Warnf("object storage unreachable: %v", err)
	} else {
		logging.Infof("object storage ready: %s/%s", cfg.Endpoint, cfg.Bucket)
	}
	return assets.NewService(store.NewAssetStore(db), backend, cfg)
}
