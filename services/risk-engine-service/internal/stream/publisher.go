package stream

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Publisher struct {
	rdb             *redis.Client
	scoresStreamKey string
	alertsStreamKey string
}

func NewPublisher(rdb *redis.Client, scoresStreamKey, alertsStreamKey string) *Publisher {
	return &Publisher{rdb: rdb, scoresStreamKey: scoresStreamKey, alertsStreamKey: alertsStreamKey}
}

// PublishScore XADDs to stream:risk-scores — every computed score, whether
// or not it crosses the alert threshold (PRD section 9).
func (p *Publisher) PublishScore(ctx context.Context, userID, locationID int64, score float64, trend string) (string, error) {
	id, err := p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: p.scoresStreamKey,
		Values: map[string]any{
			"userId":     strconv.FormatInt(userID, 10),
			"locationId": strconv.FormatInt(locationID, 10),
			"score":      strconv.FormatFloat(score, 'f', 2, 64),
			"trend":      trend,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("xadd %s: %w", p.scoresStreamKey, err)
	}
	return id, nil
}

// PublishAlert XADDs to stream:risk-alerts — only messages that passed the
// threshold + debounce check (PRD section 9 & 10).
func (p *Publisher) PublishAlert(ctx context.Context, userID, locationID int64, score float64, message string) (string, error) {
	id, err := p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: p.alertsStreamKey,
		Values: map[string]any{
			"userId":     strconv.FormatInt(userID, 10),
			"locationId": strconv.FormatInt(locationID, 10),
			"score":      strconv.FormatFloat(score, 'f', 2, 64),
			"message":    message,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("xadd %s: %w", p.alertsStreamKey, err)
	}
	return id, nil
}
