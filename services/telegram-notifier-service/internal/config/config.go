// Package config loads telegram-notifier-service configuration from
// environment variables, with .env support for local dev (same approach as
// scheduler-service / risk-engine-service).
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config holds every runtime setting the notifier needs.
type Config struct {
	HTTPPort string `env:"HTTP_PORT" envDefault:"8080"`

	RedisAddr     string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	RedisPassword string `env:"REDIS_PASSWORD" envDefault:""`
	RedisDB       int    `env:"REDIS_DB" envDefault:"0"`

	DatabaseURL string `env:"DATABASE_URL" envDefault:"postgres://jaganapas:jaganapas@localhost:5432/jaganapas?sslmode=disable"`

	// Redis Streams (PRD section 9).
	AlertsStreamKey string `env:"ALERTS_STREAM_KEY" envDefault:"stream:risk-alerts"`
	ConsumerGroup   string `env:"CONSUMER_GROUP" envDefault:"cg-telegram-notifier"`
	ConsumerName    string `env:"CONSUMER_NAME" envDefault:"telegram-notifier-1"`

	ReadBlock     time.Duration `env:"READ_BLOCK" envDefault:"5s"`
	ReadCount     int64         `env:"READ_COUNT" envDefault:"10"`
	ClaimInterval time.Duration `env:"CLAIM_INTERVAL" envDefault:"30s"`
	ClaimMinIdle  time.Duration `env:"CLAIM_MIN_IDLE" envDefault:"1m"`

	// Telegram Bot API (PRD section 7: "via go-telegram-bot-api atau webhook manual").
	TelegramBotToken   string        `env:"TELEGRAM_BOT_TOKEN" envDefault:""`
	TelegramAPIBaseURL string        `env:"TELEGRAM_API_BASE_URL" envDefault:"https://api.telegram.org"`
	TelegramTimeout    time.Duration `env:"TELEGRAM_TIMEOUT" envDefault:"10s"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	LogFile  string `env:"LOG_FILE" envDefault:"logs/app.log"`
}

// Load reads .env (if present) then parses environment variables into Config.
func Load() (*Config, error) {
	_ = godotenv.Load() // local dev convenience; ignored if the file doesn't exist

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env config: %w", err)
	}
	return cfg, nil
}
