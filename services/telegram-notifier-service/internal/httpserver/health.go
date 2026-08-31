// Package httpserver exposes /healthz (PRD section 12) on a Fiber app, same
// stack as scheduler-service / risk-engine-service.
package httpserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"

	"github.com/jaganapas/telegram-notifier-service/internal/db"
)

func New(rdb *redis.Client, database *db.DB, log *slog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "telegram-notifier-service",
		DisableStartupMessage: true,
	})

	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			log.Error("panic recovered in http handler", "path", c.Path(), "panic", e)
		},
	}))
	app.Use(requestLogger(log))

	app.Get("/healthz", healthHandler(rdb, database, log))

	return app
}

func requestLogger(log *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		elapsed := time.Since(start)

		status := c.Response().StatusCode()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		log.Log(c.Context(), level, "http request",
			"method", c.Method(),
			"path", c.Path(),
			"status", status,
			"latency_ms", elapsed.Milliseconds(),
			"ip", c.IP(),
		)
		return err
	}
}

func healthHandler(rdb *redis.Client, database *db.DB, log *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
		defer cancel()

		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Error("healthz: redis ping failed", "error", err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"service": "telegram-notifier-service",
				"status":  "redis unavailable: " + err.Error(),
			})
		}

		if err := database.Pool.Ping(ctx); err != nil {
			log.Error("healthz: postgres ping failed", "error", err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"service": "telegram-notifier-service",
				"status":  "postgres unavailable: " + err.Error(),
			})
		}

		return c.JSON(fiber.Map{
			"service": "telegram-notifier-service",
			"status":  "ok",
		})
	}
}
