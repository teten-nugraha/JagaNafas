// Package config loads scheduler-service configuration from environment
// variables (with .env support for local dev) and the bundled locations list.
package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

//go:embed locations.json
var defaultLocationsJSON []byte

// Location is a monitored city: an Open-Meteo poll target for AQI + weather.
type Location struct {
	ID   int64   `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// Config holds every runtime setting the scheduler needs.
type Config struct {
	HTTPPort          string        `env:"HTTP_PORT" envDefault:"8080"`
	RedisAddr         string        `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	RedisPassword     string        `env:"REDIS_PASSWORD" envDefault:""`
	RedisDB           int           `env:"REDIS_DB" envDefault:"0"`
	StreamKey         string        `env:"STREAM_KEY" envDefault:"stream:raw-environment-data"`
	PollInterval      time.Duration `env:"POLL_INTERVAL" envDefault:"15m"`
	OpenMeteoAQIURL   string        `env:"OPEN_METEO_AQI_URL" envDefault:"https://air-quality-api.open-meteo.com/v1/air-quality"`
	OpenMeteoWeatherURL string      `env:"OPEN_METEO_WEATHER_URL" envDefault:"https://api.open-meteo.com/v1/forecast"`
	HTTPTimeout       time.Duration `env:"HTTP_TIMEOUT" envDefault:"10s"`
	LocationsFile     string        `env:"LOCATIONS_FILE" envDefault:""`
	LogLevel          string        `env:"LOG_LEVEL" envDefault:"info"`
	LogFile           string        `env:"LOG_FILE" envDefault:"logs/app.log"`

	Locations []Location `env:"-"`
}

// Load reads .env (if present), parses environment variables into Config,
// and resolves the locations list (bundled default, or LOCATIONS_FILE override).
func Load() (*Config, error) {
	_ = godotenv.Load() // local dev convenience; ignored if the file doesn't exist

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env config: %w", err)
	}

	raw := defaultLocationsJSON
	if cfg.LocationsFile != "" {
		b, err := os.ReadFile(cfg.LocationsFile)
		if err != nil {
			return nil, fmt.Errorf("read locations file %q: %w", cfg.LocationsFile, err)
		}
		raw = b
	}

	var locations []Location
	if err := json.Unmarshal(raw, &locations); err != nil {
		return nil, fmt.Errorf("parse locations json: %w", err)
	}
	if len(locations) == 0 {
		return nil, fmt.Errorf("no locations configured")
	}
	cfg.Locations = locations

	return cfg, nil
}
