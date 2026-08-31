// Command riskengine is the JagaNapas Risk Engine service (PRD section 6 &
// 7.1): consumes stream:raw-environment-data, computes personalized risk
// scores per subscriber, publishes stream:risk-scores / stream:risk-alerts,
// and serves /healthz.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jaganapas/risk-engine-service/internal/config"
	"github.com/jaganapas/risk-engine-service/internal/consumer"
	"github.com/jaganapas/risk-engine-service/internal/db"
	"github.com/jaganapas/risk-engine-service/internal/httpserver"
	"github.com/jaganapas/risk-engine-service/internal/logging"
)

func main() {
	// The container has no shell/curl/wget (distroless), so Docker's
	// HEALTHCHECK runs this binary with -healthcheck instead: a quick
	// self GET against /healthz that exits 0/1.
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(runHealthcheck())
	}

	bootLog := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		bootLog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	log, logFile, err := logging.New(cfg.LogFile, cfg.LogLevel)
	if err != nil {
		bootLog.Error("logging init failed", "error", err)
		os.Exit(1)
	}
	defer logFile.Close()
	slog.SetDefault(log)

	log.Info("config loaded",
		"http_port", cfg.HTTPPort,
		"redis_addr", cfg.RedisAddr,
		"raw_stream_key", cfg.RawStreamKey,
		"scores_stream_key", cfg.ScoresStreamKey,
		"alerts_stream_key", cfg.AlertsStreamKey,
		"consumer_group", cfg.ConsumerGroup,
		"consumer_name", cfg.ConsumerName,
		"trend_window", cfg.TrendWindow.String(),
		"debounce_window", cfg.DebounceWindow.String(),
		"log_level", cfg.LogLevel,
		"log_file", cfg.LogFile,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Error("redis client close error", "error", err)
		}
	}()

	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	err = rdb.Ping(pingCtx).Err()
	cancelPing()
	if err != nil {
		log.Error("redis ping failed on startup", "redis_addr", cfg.RedisAddr, "error", err)
		os.Exit(1)
	}
	log.Info("redis connected", "redis_addr", cfg.RedisAddr)

	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	log.Info("postgres connected")

	if err := database.Migrate(ctx); err != nil {
		log.Error("postgres schema migration failed", "error", err)
		os.Exit(1)
	}
	log.Info("postgres schema ready")

	cons := consumer.New(cfg, rdb, database, log)
	if err := cons.EnsureGroup(ctx); err != nil {
		log.Error("ensure consumer group failed", "error", err)
		os.Exit(1)
	}
	log.Info("consumer group ready", "group", cfg.ConsumerGroup, "stream", cfg.RawStreamKey)

	app := httpserver.New(rdb, database, log)
	addr := ":" + cfg.HTTPPort

	go func() {
		log.Info("http server listening", "addr", addr)
		if err := app.Listen(addr); err != nil {
			log.Error("http server stopped unexpectedly", "error", err)
			stop()
		}
	}()

	go cons.Run(ctx)

	<-ctx.Done()
	log.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Error("http server shutdown error", "error", err)
	} else {
		log.Info("http server shut down cleanly")
	}

	log.Info("risk engine stopped")
}

// runHealthcheck backs `riskengine -healthcheck`, used by Docker's
// HEALTHCHECK since the distroless image has no curl/wget/shell.
func runHealthcheck() int {
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck request failed:", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck non-200 status:", resp.StatusCode)
		return 1
	}
	return 0
}
