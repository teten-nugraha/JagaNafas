// Package openmeteo resolves a city name to coordinates via the free
// Open-Meteo Geocoding API (PRD section 5 step 3: "geocoding via Open-Meteo").
package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

type GeocodeClient struct {
	baseURL    string
	httpClient *http.Client
	log        *slog.Logger
}

func NewGeocodeClient(baseURL string, timeout time.Duration, log *slog.Logger) *GeocodeClient {
	return &GeocodeClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
		log:        log,
	}
}

// Place is a resolved location: coordinates plus a human-readable display
// name ("Bandung, Jawa Barat"), matching the PRD section 11 message format.
type Place struct {
	DisplayName string
	Lat         float64
	Lon         float64
}

type geocodeResponse struct {
	Results []struct {
		Name      string  `json:"name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Admin1    string  `json:"admin1"`
		Country   string  `json:"country"`
	} `json:"results"`
}

// ErrNoMatch means the query matched no place — the caller should ask the
// user to try a different / more specific name.
var ErrNoMatch = fmt.Errorf("no matching location")

// Search resolves a free-text city name to its best-matching place.
func (c *GeocodeClient) Search(ctx context.Context, query string) (Place, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return Place{}, fmt.Errorf("parse base url: %w", err)
	}
	u.RawQuery = url.Values{
		"name":     {query},
		"count":    {"1"},
		"language": {"id"},
		"format":   {"json"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Place{}, fmt.Errorf("build request: %w", err)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return Place{}, fmt.Errorf("do geocode request: %w", err)
	}
	defer resp.Body.Close()

	c.log.Debug("open-meteo geocode request", "query", query, "status", resp.StatusCode, "elapsed_ms", elapsed.Milliseconds())

	if resp.StatusCode != http.StatusOK {
		return Place{}, fmt.Errorf("unexpected status %d from geocoding API", resp.StatusCode)
	}

	var parsed geocodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Place{}, fmt.Errorf("decode geocode response: %w", err)
	}

	if len(parsed.Results) == 0 {
		return Place{}, ErrNoMatch
	}

	r := parsed.Results[0]
	name := r.Name
	if r.Admin1 != "" {
		name = fmt.Sprintf("%s, %s", r.Name, r.Admin1)
	}

	return Place{DisplayName: name, Lat: r.Latitude, Lon: r.Longitude}, nil
}
