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
	"github.com/liuscraft/orion-x/internal/metrics"
	"github.com/liuscraft/orion-x/internal/wsserver"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	configPath := flag.String("config", config.DefaultPath, "config file path")
	flag.Parse()

	appConfig, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	if err := appConfig.ValidateKeys(true, true, true); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid config: %v\n", err)
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

	var wsMetrics *metrics.WSServerMetrics
	var voicebotMetrics *metrics.VoicebotMetrics
	var metricsServer *http.Server

	if appConfig.Metrics.Enabled {
		reg := metrics.NewRegistry()
		wsMetrics = metrics.NewWSServerMetrics(reg)
		voicebotMetrics = metrics.NewVoicebotMetrics(reg)

		metricsHandler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
			Registry:            reg,
			EnableOpenMetrics:   appConfig.Metrics.EnableOpenMetrics,
			MaxRequestsInFlight: appConfig.Metrics.MaxRequestsInFlight,
		})
		if token := strings.TrimSpace(appConfig.Metrics.BearerToken); token != "" {
			metricsHandler = withBearerAuth(metricsHandler, token)
		}

		metricsMux := http.NewServeMux()
		metricsMux.Handle(appConfig.Metrics.Path, metricsHandler)
		metricsServer = &http.Server{
			Addr:    appConfig.Metrics.Address,
			Handler: metricsMux,
		}

		go func() {
			logging.Infof("Metrics server listening on %s%s", appConfig.Metrics.Address, appConfig.Metrics.Path)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logging.Fatalf("Metrics server error: %v", err)
			}
		}()
	}

	server := wsserver.NewServer(appConfig, wsMetrics, voicebotMetrics)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel()
	}()

	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			logging.Fatalf("WebSocket server error: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	if metricsServer != nil {
		_ = metricsServer.Shutdown(shutdownCtx)
	}
}

func withBearerAuth(handler http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		const prefix = "bearer "
		if !strings.HasPrefix(strings.ToLower(auth), prefix) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		got := strings.TrimSpace(auth[len(prefix):])
		if got != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	})
}
