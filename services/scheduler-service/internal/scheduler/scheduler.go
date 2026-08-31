// Package scheduler runs the periodic poll-and-publish loop described in
// PRD section 6: every pollInterval, fetch each configured location from
// Open-Meteo and publish the reading onto stream:raw-environment-data.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jaganapas/scheduler-service/internal/config"
	"github.com/jaganapas/scheduler-service/internal/openmeteo"
	"github.com/jaganapas/scheduler-service/internal/stream"
)

type Scheduler struct {
	locations []config.Location
	interval  time.Duration
	client    *openmeteo.Client
	publisher *stream.Publisher
	log       *slog.Logger
}

func New(locations []config.Location, interval time.Duration, client *openmeteo.Client, publisher *stream.Publisher, log *slog.Logger) *Scheduler {
	return &Scheduler{
		locations: locations,
		interval:  interval,
		client:    client,
		publisher: publisher,
		log:       log,
	}
}

// Run polls immediately, then on every tick, until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	s.log.Info("scheduler loop starting", "poll_interval", s.interval.String(), "locations", len(s.locations))

	s.pollAll(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.log.Info("scheduler loop stopping", "reason", ctx.Err())
			return
		case <-ticker.C:
			s.pollAll(ctx)
		}
	}
}

// pollAll fetches and publishes every location concurrently; one location's
// failure is logged but never blocks the others (NFR: <2min end-to-end latency).
func (s *Scheduler) pollAll(ctx context.Context) {
	start := time.Now()
	s.log.Debug("poll cycle started", "locations", len(s.locations))

	var wg sync.WaitGroup
	var okCount, failCount int32
	var mu sync.Mutex

	for _, loc := range s.locations {
		wg.Add(1)
		go func(loc config.Location) {
			defer wg.Done()
			if s.pollOne(ctx, loc) {
				mu.Lock()
				okCount++
				mu.Unlock()
			} else {
				mu.Lock()
				failCount++
				mu.Unlock()
			}
		}(loc)
	}

	wg.Wait()

	elapsed := time.Since(start)
	logFn := s.log.Info
	if failCount > 0 {
		logFn = s.log.Warn
	}
	logFn("poll cycle complete",
		"locations", len(s.locations),
		"succeeded", okCount,
		"failed", failCount,
		"elapsed_ms", elapsed.Milliseconds(),
	)
}

// pollOne fetches and publishes a single location's reading, returning
// whether it succeeded. Every failure branch logs enough context (location,
// error, timing) to troubleshoot without needing to reproduce the issue.
func (s *Scheduler) pollOne(ctx context.Context, loc config.Location) bool {
	fetchStart := time.Now()
	reading, err := s.client.Fetch(ctx, loc.Lat, loc.Lon)
	fetchElapsed := time.Since(fetchStart)

	if err != nil {
		s.log.Error("open-meteo fetch failed",
			"location_id", loc.ID,
			"location_name", loc.Name,
			"lat", loc.Lat,
			"lon", loc.Lon,
			"elapsed_ms", fetchElapsed.Milliseconds(),
			"error", err,
		)
		return false
	}
	s.log.Debug("open-meteo fetch ok",
		"location_id", loc.ID,
		"location_name", loc.Name,
		"elapsed_ms", fetchElapsed.Milliseconds(),
	)

	publishStart := time.Now()
	streamID, err := s.publisher.PublishReading(ctx, loc.ID, reading)
	publishElapsed := time.Since(publishStart)

	if err != nil {
		s.log.Error("redis stream publish failed",
			"location_id", loc.ID,
			"location_name", loc.Name,
			"elapsed_ms", publishElapsed.Milliseconds(),
			"error", err,
		)
		return false
	}

	s.log.Info("reading published",
		"location_id", loc.ID,
		"location_name", loc.Name,
		"stream_id", streamID,
		"pm25", reading.PM25,
		"pm10", reading.PM10,
		"temp", reading.Temperature,
		"humidity", reading.Humidity,
		"observed_at", reading.ObservedAt.Format(time.RFC3339),
		"fetch_ms", fetchElapsed.Milliseconds(),
		"publish_ms", publishElapsed.Milliseconds(),
	)
	return true
}
