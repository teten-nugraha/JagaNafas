// Package httpserver exposes the /healthz endpoint (PRD section 12) on a
// Fiber app, with structured request logging and panic recovery so HTTP
// issues show up in the same troubleshooting log as the rest of the service.
package httpserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"
)

func New(rdb *redis.Client, log *slog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "scheduler-service",
		DisableStartupMessage: true,
	})

	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			log.Error("panic recovered in http handler", "path", c.Path(), "panic", e)
		},
	}))
	app.Use(requestLogger(log))

	app.Get("/healthz", healthHandler(rdb, log))

	return app
}

// requestLogger logs every request after it completes: method, path,
// status, latency and client IP — the minimum needed to debug a slow or
// failing /healthz probe without reproducing it live.
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

func healthHandler(rdb *redis.Client, log *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
		defer cancel()

		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Error("healthz: redis ping failed", "error", err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"service": "scheduler-service",
				"status":  "redis unavailable: " + err.Error(),
			})
		}

		return c.JSON(fiber.Map{
			"service": "scheduler-service",
			"status":  "ok",
		})
	}
}
