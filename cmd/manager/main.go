package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/manager/app"
	"github.com/liuscraft/orion-x/internal/manager/auth"
	"github.com/liuscraft/orion-x/internal/manager/contracts"
	"github.com/liuscraft/orion-x/internal/manager/httpapi"
	"github.com/liuscraft/orion-x/internal/manager/platformresource"
	"github.com/liuscraft/orion-x/internal/manager/providertemplate"
	"github.com/liuscraft/orion-x/internal/manager/security"
	"github.com/liuscraft/orion-x/internal/manager/storage"
	"github.com/liuscraft/orion-x/internal/manager/toolentitlement"
	"github.com/liuscraft/orion-x/internal/manager/toolmarket"
	"github.com/liuscraft/orion-x/internal/manager/toolruntime"
	"github.com/liuscraft/orion-x/internal/manager/toolvalidator"
)

func main() {
	configPath := flag.String("config", config.DefaultManagerPath, "manager config file path")
	migrateOnly := flag.Bool("migrate-only", false, "run database migrations and exit")
	flag.Parse()

	appConfig, err := config.LoadManager(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load manager config: %v\n", err)
		os.Exit(1)
	}

	if err := logging.Init(logging.Config{
		Level:  appConfig.Logging.Level,
		Format: appConfig.Logging.Format,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logging.Sync()

	logging.SetTraceID(logging.NewTraceID())

	ctx := context.Background()
	store, err := storage.OpenPostgres(ctx, appConfig.Database)
	if err != nil {
		logging.Fatalf("Open manager database failed: %v", err)
	}
	defer store.Close()
	logging.Infof("Manager database connection status: healthy")

	migrator := storage.NewMigrator(store.DB())
	healthHandler := httpapi.NewHealthHandler(store, time.Duration(appConfig.Database.PingTimeoutMs)*time.Millisecond)
	authTokenManager, err := auth.NewJWTManager(auth.JWTManagerConfig{
		Secret:     appConfig.Auth.JWTSecret,
		Issuer:     appConfig.Auth.Issuer,
		AccessTTL:  time.Duration(appConfig.Auth.AccessTokenTTLSeconds) * time.Second,
		RefreshTTL: time.Duration(appConfig.Auth.RefreshTokenTTLSeconds) * time.Second,
	})
	if err != nil {
		logging.Fatalf("Init manager auth token manager failed: %v", err)
	}
	userRepository := storage.NewAuthUserRepository(store.DB())
	authService := auth.NewService(userRepository, authTokenManager)
	authHandler := httpapi.NewAuthHandler(authService)
	authMiddleware := httpapi.NewAuthMiddleware(authService)
	accessKeyCipher, err := security.NewAESCipher(appConfig.Security.AccessKeyCipherSecret)
	if err != nil {
		logging.Fatalf("Init manager access key cipher failed: %v", err)
	}
	platformResourceRepository := storage.NewPlatformResourceRepository(store.DB())
	platformResourceService := platformresource.NewService(platformResourceRepository, accessKeyCipher)
	platformResourceHandler := httpapi.NewPlatformResourceHandler(platformResourceService, authService)
	providerTemplateRepository := storage.NewProviderTemplateRepository(store.DB())
	providerTemplateService := providertemplate.NewService(providerTemplateRepository)
	providerTemplateHandler := httpapi.NewProviderTemplateHandler(providerTemplateService)
	toolMarketRepository := storage.NewToolMarketRepository(store.DB())
	toolEntitlementRepository := storage.NewToolEntitlementRepository(store.DB())
	toolMarketService := toolmarket.NewService(toolMarketRepository, toolvalidator.NewMCPConfigValidator())
	toolEntitlementService := toolentitlement.NewService(toolEntitlementRepository, toolMarketRepository, userRepository)
	toolRuntimeService := toolruntime.NewService(toolEntitlementService)
	toolMarketHandler := httpapi.NewToolMarketHandler(toolMarketService, toolEntitlementService)
	toolEntitlementHandler := httpapi.NewToolEntitlementHandler(toolEntitlementService)
	toolEntitlementHandler.SetRuntimeService(toolRuntimeService)

	router := http.NewServeMux()
	router.Handle(appConfig.Server.HealthPath, healthHandler)
	router.Handle("/api/v1/auth/register", http.HandlerFunc(authHandler.Register))
	router.Handle("/api/v1/auth/login", http.HandlerFunc(authHandler.Login))
	router.Handle("/api/v1/auth/refresh", http.HandlerFunc(authHandler.Refresh))
	router.Handle("/api/v1/auth/logout", authMiddleware.RequireAuth(http.HandlerFunc(authHandler.Logout)))
	router.Handle(
		"/api/v1/admin/platform-resources",
		authMiddleware.RequireAuth(authMiddleware.RequireRole(contracts.RoleAdmin)(http.HandlerFunc(platformResourceHandler.Create))),
	)
	router.Handle(
		"/api/v1/admin/platform-resources/",
		authMiddleware.RequireAuth(authMiddleware.RequireRole(contracts.RoleAdmin)(http.HandlerFunc(platformResourceHandler.ByID))),
	)
	router.Handle("/api/v1/platform-resources", authMiddleware.RequireAuth(http.HandlerFunc(platformResourceHandler.List)))
	router.Handle(
		"/api/v1/admin/provider-templates",
		authMiddleware.RequireAuth(authMiddleware.RequireRole(contracts.RoleAdmin)(http.HandlerFunc(providerTemplateHandler.Create))),
	)
	router.Handle(
		"/api/v1/admin/provider-templates/",
		authMiddleware.RequireAuth(authMiddleware.RequireRole(contracts.RoleAdmin)(http.HandlerFunc(providerTemplateHandler.ByID))),
	)
	router.Handle("/api/v1/provider-templates", authMiddleware.RequireAuth(http.HandlerFunc(providerTemplateHandler.List)))
	router.Handle(
		"/api/v1/admin/tool-market/items",
		authMiddleware.RequireAuth(authMiddleware.RequireRole(contracts.RoleAdmin)(http.HandlerFunc(toolMarketHandler.AdminItems))),
	)
	router.Handle(
		"/api/v1/admin/tool-market/items/",
		authMiddleware.RequireAuth(authMiddleware.RequireRole(contracts.RoleAdmin)(http.HandlerFunc(toolMarketHandler.AdminByItem))),
	)
	router.Handle(
		"/api/v1/admin/tool-entitlements/grant",
		authMiddleware.RequireAuth(authMiddleware.RequireRole(contracts.RoleAdmin)(http.HandlerFunc(toolEntitlementHandler.Grant))),
	)
	router.Handle("/api/v1/tool-market/items", authMiddleware.RequireAuth(http.HandlerFunc(toolMarketHandler.PublicItems)))
	router.Handle("/api/v1/tool-market/items/", authMiddleware.RequireAuth(http.HandlerFunc(toolMarketHandler.PublicByItem)))
	router.Handle("/api/v1/me/tool-repo", authMiddleware.RequireAuth(http.HandlerFunc(toolEntitlementHandler.ListRepo)))
	router.Handle("/api/v1/me/tool-repo/", authMiddleware.RequireAuth(http.HandlerFunc(toolEntitlementHandler.RepoByID)))

	server := httpapi.NewServer(appConfig.Server, router)
	lifecycle := app.NewLifecycle(appConfig.Migration.AutoMigrateOnStartup, migrator, server)

	migrationResult, err := lifecycle.Bootstrap(ctx, *migrateOnly)
	if err != nil {
		logging.Fatalf("Manager bootstrap failed: %v", err)
	}

	if appConfig.Migration.AutoMigrateOnStartup || *migrateOnly {
		logging.Infof(
			"Manager migration version=%d previous_version=%d applied=%t tables=%s",
			migrationResult.TargetVersion,
			migrationResult.PreviousVersion,
			migrationResult.Applied,
			strings.Join(migrationResult.Tables, ","),
		)
	} else {
		logging.Infof("Manager migration skipped on startup")
	}

	if *migrateOnly {
		logging.Infof("Manager migrate-only completed")
		return
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- lifecycle.Start()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrCh:
		if err != nil {
			logging.Fatalf("Manager HTTP server error: %v", err)
		}
		return
	case <-sigCh:
		logging.Infof("Manager shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := lifecycle.Shutdown(shutdownCtx); err != nil {
		logging.Errorf("Manager shutdown error: %v", err)
	}
	if err := <-serverErrCh; err != nil {
		logging.Errorf("Manager HTTP server stop error: %v", err)
	}
}
