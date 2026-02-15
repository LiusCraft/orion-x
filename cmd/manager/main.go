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

	"github.com/liuscraft/orion-x/internal/config"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/manager/app"
	"github.com/liuscraft/orion-x/internal/manager/httpapi"
	"github.com/liuscraft/orion-x/internal/manager/storage"
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
	server := httpapi.NewServer(appConfig.Server, healthHandler)
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
