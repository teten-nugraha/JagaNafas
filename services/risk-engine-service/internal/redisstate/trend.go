// Package redisstate implements the manual state management the PRD calls
// for in place of a managed stream-processing state store (section 7 note):
// rolling trend windows, alert debounce, and score cache — all in Redis.
package redisstate

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type TrendStore struct {
	rdb    *redis.Client
	window time.Duration
}

func NewTrendStore(rdb *redis.Client, window time.Duration) *TrendStore {
	return &TrendStore{rdb: rdb, window: window}
}

func trendKey(locationID int64) string {
	return fmt.Sprintf("trend:pm25:%d", locationID)
}

// Trend records the current PM2.5 reading in a Sorted Set scored by unix
// timestamp, prunes entries older than the rolling window, and returns the
// average of everything else still in the window (PRD section 10 step 3:
// "disimpan sebagai Redis Sorted Set ... entry lama dibuang dgn ZREMRANGEBYSCORE").
// avgAvailable is false when this is the first reading for the location.
func (t *TrendStore) Record(ctx context.Context, locationID int64, pm25 float64, observedAt time.Time) (avg float64, avgAvailable bool, err error) {
	key := trendKey(locationID)
	now := observedAt.Unix()
	cutoff := observedAt.Add(-t.window).Unix()

	if err := t.rdb.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("(%d", cutoff)).Err(); err != nil {
		return 0, false, fmt.Errorf("prune trend window: %w", err)
	}

	existing, err := t.rdb.ZRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", cutoff),
		Max: fmt.Sprintf("%d", now),
	}).Result()
	if err != nil {
		return 0, false, fmt.Errorf("read trend window: %w", err)
	}

	avg, avgAvailable, err = t.average(existing)
	if err != nil {
		return 0, false, err
	}

	member := redis.Z{Score: float64(now), Member: fmt.Sprintf("%d:%f", now, pm25)}
	if err := t.rdb.ZAdd(ctx, key, member).Err(); err != nil {
		return 0, false, fmt.Errorf("record trend point: %w", err)
	}
	// keep the key from growing unbounded even under clock skew / retries
	t.rdb.Expire(ctx, key, t.window+time.Hour)

	return avg, avgAvailable, nil
}

func (t *TrendStore) average(points []redis.Z) (float64, bool, error) {
	if len(points) == 0 {
		return 0, false, nil
	}
	var sum float64
	var n int
	for _, p := range points {
		member, ok := p.Member.(string)
		if !ok {
			continue
		}
		var ts int64
		var val float64
		if _, err := fmt.Sscanf(member, "%d:%f", &ts, &val); err != nil {
			continue
		}
		sum += val
		n++
	}
	if n == 0 {
		return 0, false, nil
	}
	return sum / float64(n), true, nil
}
