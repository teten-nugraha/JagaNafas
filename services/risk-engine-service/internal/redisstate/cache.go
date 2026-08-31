package redisstate

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type ScoreCache struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewScoreCache builds a cache with a generous TTL so a stale entry expires
// on its own if the pipeline stalls, rather than serving a score forever.
func NewScoreCache(rdb *redis.Client, ttl time.Duration) *ScoreCache {
	return &ScoreCache{rdb: rdb, ttl: ttl}
}

func locationKey(locationID int64) string {
	return fmt.Sprintf("cache:score:location:%d", locationID)
}

func userKey(userID int64) string {
	return fmt.Sprintf("cache:score:user:%d", userID)
}

type Entry struct {
	LocationID int64
	Score      float64
	Category   string
	Trend      string
	PM25       float64
	PM10       float64
	Temp       float64
	Humidity   float64
	ComputedAt time.Time
}

func (e Entry) toFields() map[string]any {
	return map[string]any{
		"location_id": strconv.FormatInt(e.LocationID, 10),
		"score":       strconv.FormatFloat(e.Score, 'f', 2, 64),
		"category":    e.Category,
		"trend":       e.Trend,
		"pm25":        strconv.FormatFloat(e.PM25, 'f', 2, 64),
		"pm10":        strconv.FormatFloat(e.PM10, 'f', 2, 64),
		"temp":        strconv.FormatFloat(e.Temp, 'f', 2, 64),
		"humidity":    strconv.FormatFloat(e.Humidity, 'f', 2, 64),
		"computed_at": e.ComputedAt.Format(time.RFC3339),
	}
}

// SetLocation caches a location-level representative score (general
// sensitivity, multiplier 1.0), used by the internal admin API
// `/api/locations/{id}/current` (PRD section 12).
func (c *ScoreCache) SetLocation(ctx context.Context, e Entry) error {
	key := locationKey(e.LocationID)
	if err := c.rdb.HSet(ctx, key, e.toFields()).Err(); err != nil {
		return fmt.Errorf("cache location score: %w", err)
	}
	return c.rdb.Expire(ctx, key, c.ttl).Err()
}

// SetUser caches one user's personalized latest score, used by the bot's
// `/status` command (PRD section 6: "baca Redis untuk /status").
func (c *ScoreCache) SetUser(ctx context.Context, userID int64, e Entry) error {
	key := userKey(userID)
	if err := c.rdb.HSet(ctx, key, e.toFields()).Err(); err != nil {
		return fmt.Errorf("cache user score: %w", err)
	}
	return c.rdb.Expire(ctx, key, c.ttl).Err()
}
