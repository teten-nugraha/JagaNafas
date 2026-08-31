package redisstate

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// CachedScore mirrors the Hash risk-engine-service writes to
// cache:score:user:{userId} (PRD section 6: "baca Redis untuk /status").
type CachedScore struct {
	LocationID int64
	Score      float64
	Category   string
	Trend      string
	PM25       float64
	ComputedAt string
}

type ScoreCacheReader struct {
	rdb *redis.Client
}

func NewScoreCacheReader(rdb *redis.Client) *ScoreCacheReader {
	return &ScoreCacheReader{rdb: rdb}
}

// ErrNoCachedScore means risk-engine-service hasn't computed a score for
// this user yet (e.g. they just subscribed and the next poll hasn't run).
var ErrNoCachedScore = fmt.Errorf("no cached score")

func (r *ScoreCacheReader) Get(ctx context.Context, userID int64) (CachedScore, error) {
	key := fmt.Sprintf("cache:score:user:%d", userID)
	vals, err := r.rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return CachedScore{}, fmt.Errorf("get cached score: %w", err)
	}
	if len(vals) == 0 {
		return CachedScore{}, ErrNoCachedScore
	}

	var cs CachedScore
	fmt.Sscanf(vals["location_id"], "%d", &cs.LocationID)
	fmt.Sscanf(vals["score"], "%f", &cs.Score)
	fmt.Sscanf(vals["pm25"], "%f", &cs.PM25)
	cs.Category = vals["category"]
	cs.Trend = vals["trend"]
	cs.ComputedAt = vals["computed_at"]
	return cs, nil
}
