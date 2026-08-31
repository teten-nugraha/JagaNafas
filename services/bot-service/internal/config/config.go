// Package config loads bot-service configuration from environment
// variables, with .env support for local dev (same approach as the other
// JagaNapas services).
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort string `env:"HTTP_PORT" envDefault:"8080"`

	RedisAddr     string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	RedisPassword string `env:"REDIS_PASSWORD" envDefault:""`
	RedisDB       int    `env:"REDIS_DB" envDefault:"0"`

	DatabaseURL string `env:"DATABASE_URL" envDefault:"postgres://jaganapas:jaganapas@localhost:5432/jaganapas?sslmode=disable"`

	// Telegram Bot API (PRD section 7: "via go-telegram-bot-api atau webhook
	// manual"). This service uses long polling (getUpdates) — no public
	// HTTPS endpoint needed, which keeps local/compose deployment simple.
	TelegramBotToken   string        `env:"TELEGRAM_BOT_TOKEN" envDefault:""`
	TelegramAPIBaseURL string        `env:"TELEGRAM_API_BASE_URL" envDefault:"https://api.telegram.org"`
	PollTimeoutSec     int           `env:"TELEGRAM_POLL_TIMEOUT_SEC" envDefault:"30"`
	HTTPClientTimeout  time.Duration `env:"TELEGRAM_HTTP_TIMEOUT" envDefault:"40s"`

	OpenMeteoGeocodingURL string        `env:"OPEN_METEO_GEOCODING_URL" envDefault:"https://geocoding-api.open-meteo.com/v1/search"`
	OpenMeteoHTTPTimeout  time.Duration `env:"OPEN_METEO_HTTP_TIMEOUT" envDefault:"10s"`

	// FSM state per chat (internal/redisstate) — how long an in-progress
	// onboarding/edit flow survives before it's considered abandoned.
	SessionTTL time.Duration `env:"SESSION_TTL" envDefault:"15m"`

	// /riwayat window (PRD section 5: "lihat 7 hari terakhir").
	HistoryDays  int `env:"HISTORY_DAYS" envDefault:"7"`
	HistoryLimit int `env:"HISTORY_LIMIT" envDefault:"10"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	LogFile  string `env:"LOG_FILE" envDefault:"logs/app.log"`
}

func Load() (*Config, error) {
	_ = godotenv.Load() // local dev convenience; ignored if the file doesn't exist

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env config: %w", err)
	}
	return cfg, nil
}
