// Package openmeteo fetches current air-quality and weather readings for a
// single location from the free Open-Meteo APIs (PRD section 6 & 10 input).
package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Reading is one enriched environment sample for a location, ready to be
// published to stream:raw-environment-data (PRD section 9).
type Reading struct {
	PM25        float64
	PM10        float64
	Temperature float64
	Humidity    float64
	ObservedAt  time.Time
}

type Client struct {
	AQIBaseURL     string
	WeatherBaseURL string
	HTTPClient     *http.Client
	Log            *slog.Logger
}

func NewClient(aqiBaseURL, weatherBaseURL string, timeout time.Duration, log *slog.Logger) *Client {
	return &Client{
		AQIBaseURL:     aqiBaseURL,
		WeatherBaseURL: weatherBaseURL,
		HTTPClient:     &http.Client{Timeout: timeout},
		Log:            log,
	}
}

type aqiResponse struct {
	Current struct {
		Time  string  `json:"time"`
		PM25  float64 `json:"pm2_5"`
		PM10  float64 `json:"pm10"`
	} `json:"current"`
}

type weatherResponse struct {
	Current struct {
		Time        string  `json:"time"`
		Temperature float64 `json:"temperature_2m"`
		Humidity    float64 `json:"relative_humidity_2m"`
	} `json:"current"`
}

// Fetch pulls PM2.5/PM10 and temperature/humidity for (lat, lon), fetching
// both Open-Meteo endpoints concurrently and merging the result.
func (c *Client) Fetch(ctx context.Context, lat, lon float64) (Reading, error) {
	type result struct {
		aqi *aqiResponse
		wx  *weatherResponse
		err error
	}

	aqiCh := make(chan result, 1)
	wxCh := make(chan result, 1)

	go func() {
		r, err := fetchJSON[aqiResponse](ctx, c.HTTPClient, c.Log, c.AQIBaseURL, url.Values{
			"latitude":  {fmtFloat(lat)},
			"longitude": {fmtFloat(lon)},
			"current":   {"pm10,pm2_5"},
		})
		aqiCh <- result{aqi: r, err: err}
	}()

	go func() {
		r, err := fetchJSON[weatherResponse](ctx, c.HTTPClient, c.Log, c.WeatherBaseURL, url.Values{
			"latitude":  {fmtFloat(lat)},
			"longitude": {fmtFloat(lon)},
			"current":   {"temperature_2m,relative_humidity_2m"},
		})
		wxCh <- result{wx: r, err: err}
	}()

	aqiRes, wxRes := <-aqiCh, <-wxCh
	if aqiRes.err != nil {
		return Reading{}, fmt.Errorf("fetch air quality: %w", aqiRes.err)
	}
	if wxRes.err != nil {
		return Reading{}, fmt.Errorf("fetch weather: %w", wxRes.err)
	}

	observedAt, err := time.Parse("2006-01-02T15:04", aqiRes.aqi.Current.Time)
	if err != nil {
		observedAt = time.Now().UTC()
	}

	return Reading{
		PM25:        aqiRes.aqi.Current.PM25,
		PM10:        aqiRes.aqi.Current.PM10,
		Temperature: wxRes.wx.Current.Temperature,
		Humidity:    wxRes.wx.Current.Humidity,
		ObservedAt:  observedAt,
	}, nil
}

func fetchJSON[T any](ctx context.Context, client *http.Client, log *slog.Logger, baseURL string, query url.Values) (*T, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		logDebug(log, "open-meteo http request failed", "host", u.Host, "path", u.Path, "elapsed_ms", elapsed.Milliseconds(), "error", err)
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	logDebug(log, "open-meteo http request", "host", u.Host, "path", u.Path, "status", resp.StatusCode, "elapsed_ms", elapsed.Milliseconds())

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, u.Host, string(body))
	}

	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

func logDebug(log *slog.Logger, msg string, args ...any) {
	if log != nil {
		log.Debug(msg, args...)
	}
}

func fmtFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 6, 64)
}
