// Package stream publishes raw environment readings onto the Redis Stream
// consumed by the risk-engine service (PRD section 9: stream:raw-environment-data).
package stream

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jaganapas/scheduler-service/internal/openmeteo"
)

type Publisher struct {
	rdb       *redis.Client
	streamKey string
}

func NewPublisher(rdb *redis.Client, streamKey string) *Publisher {
	return &Publisher{rdb: rdb, streamKey: streamKey}
}

// PublishReading XADDs one enriched reading with the field layout the risk
// engine expects: locationId, pm25, pm10, temp, humidity, timestamp. It
// returns the Redis-assigned stream entry ID, useful for troubleshooting
// (correlating a specific publish with what the risk engine consumed).
func (p *Publisher) PublishReading(ctx context.Context, locationID int64, r openmeteo.Reading) (string, error) {
	id, err := p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: p.streamKey,
		Values: map[string]any{
			"locationId": strconv.FormatInt(locationID, 10),
			"pm25":       strconv.FormatFloat(r.PM25, 'f', 2, 64),
			"pm10":       strconv.FormatFloat(r.PM10, 'f', 2, 64),
			"temp":       strconv.FormatFloat(r.Temperature, 'f', 2, 64),
			"humidity":   strconv.FormatFloat(r.Humidity, 'f', 2, 64),
			"timestamp":  r.ObservedAt.Format(time.RFC3339),
		},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("xadd %s: %w", p.streamKey, err)
	}
	return id, nil
}
