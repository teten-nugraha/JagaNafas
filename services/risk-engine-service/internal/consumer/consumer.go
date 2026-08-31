// Package consumer runs the Redis Streams consumer group loop described in
// PRD section 6 & 9: read raw-environment-data, enrich + score + debounce,
// publish risk-scores/risk-alerts, and XACK — with periodic XAUTOCLAIM so a
// crash before ACK doesn't lose messages (PRD section 13 idempotency).
package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jaganapas/risk-engine-service/internal/config"
	"github.com/jaganapas/risk-engine-service/internal/db"
	"github.com/jaganapas/risk-engine-service/internal/redisstate"
	"github.com/jaganapas/risk-engine-service/internal/riskscore"
	"github.com/jaganapas/risk-engine-service/internal/stream"
)

type Consumer struct {
	rdb *redis.Client
	db  *db.DB
	log *slog.Logger

	rawStreamKey  string
	consumerGroup string
	consumerName  string
	readBlock     time.Duration
	readCount     int64
	claimInterval time.Duration
	claimMinIdle  time.Duration

	trend     *redisstate.TrendStore
	debounce  *redisstate.DebounceStore
	cache     *redisstate.ScoreCache
	publisher *stream.Publisher
}

func New(cfg *config.Config, rdb *redis.Client, database *db.DB, log *slog.Logger) *Consumer {
	return &Consumer{
		rdb:           rdb,
		db:            database,
		log:           log,
		rawStreamKey:  cfg.RawStreamKey,
		consumerGroup: cfg.ConsumerGroup,
		consumerName:  cfg.ConsumerName,
		readBlock:     cfg.ReadBlock,
		readCount:     cfg.ReadCount,
		claimInterval: cfg.ClaimInterval,
		claimMinIdle:  cfg.ClaimMinIdle,
		trend:         redisstate.NewTrendStore(rdb, cfg.TrendWindow),
		debounce:      redisstate.NewDebounceStore(rdb, cfg.DebounceWindow),
		cache:         redisstate.NewScoreCache(rdb, cfg.TrendWindow+time.Hour),
		publisher:     stream.NewPublisher(rdb, cfg.ScoresStreamKey, cfg.AlertsStreamKey),
	}
}

// EnsureGroup creates the consumer group (and the stream, if it doesn't
// exist yet) starting from the beginning of the stream. Safe to call every
// startup — BUSYGROUP just means it already exists.
func (c *Consumer) EnsureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.rawStreamKey, c.consumerGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("create consumer group: %w", err)
	}
	return nil
}

// Run starts the read loop and the pending-entry reclaim sweep, blocking
// until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) {
	go c.claimLoop(ctx)
	c.readLoop(ctx)
}

func (c *Consumer) readLoop(ctx context.Context) {
	c.log.Info("consumer read loop starting", "stream", c.rawStreamKey, "group", c.consumerGroup, "consumer", c.consumerName)

	for {
		select {
		case <-ctx.Done():
			c.log.Info("consumer read loop stopping", "reason", ctx.Err())
			return
		default:
		}

		res, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.consumerGroup,
			Consumer: c.consumerName,
			Streams:  []string{c.rawStreamKey, ">"},
			Count:    c.readCount,
			Block:    c.readBlock,
		}).Result()

		if err != nil {
			if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) {
				continue // read timeout, nothing new — expected, just loop
			}
			c.log.Error("xreadgroup failed", "error", err)
			time.Sleep(time.Second) // avoid a tight error loop
			continue
		}

		for _, s := range res {
			for _, msg := range s.Messages {
				c.handleMessage(ctx, msg)
			}
		}
	}
}

// claimLoop periodically reclaims messages that have been pending (delivered
// but never ACKed) for longer than claimMinIdle — e.g. this or another
// consumer crashed mid-processing — and reprocesses them.
func (c *Consumer) claimLoop(ctx context.Context) {
	ticker := time.NewTicker(c.claimInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.claimOnce(ctx)
		}
	}
}

func (c *Consumer) claimOnce(ctx context.Context) {
	start := "0-0"
	for {
		msgs, next, err := c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   c.rawStreamKey,
			Group:    c.consumerGroup,
			Consumer: c.consumerName,
			MinIdle:  c.claimMinIdle,
			Start:    start,
			Count:    c.readCount,
		}).Result()
		if err != nil {
			c.log.Error("xautoclaim failed", "error", err)
			return
		}

		if len(msgs) > 0 {
			c.log.Warn("reclaimed pending messages", "count", len(msgs), "min_idle", c.claimMinIdle.String())
		}
		for _, msg := range msgs {
			c.handleMessage(ctx, msg)
		}

		if next == "0-0" || len(msgs) == 0 {
			return
		}
		start = next
	}
}

// handleMessage runs the full PRD section 10 pipeline for one raw reading
// and ACKs it once processing is done. Parse/infra failures (bad message,
// DB unreachable) skip the ACK so XAUTOCLAIM retries later; a single
// subscriber's row failing doesn't block the others or the ACK, since
// retrying the whole batch for one bad row would wedge the stream.
func (c *Consumer) handleMessage(ctx context.Context, msg redis.XMessage) {
	reading, err := stream.ParseRawReading(msg.Values)
	if err != nil {
		c.log.Error("malformed raw reading, leaving pending for review", "message_id", msg.ID, "values", msg.Values, "error", err)
		return
	}

	log := c.log.With("message_id", msg.ID, "location_id", reading.LocationID)
	log.Debug("processing raw reading", "pm25", reading.PM25, "pm10", reading.PM10)

	avg, avgAvailable, err := c.trend.Record(ctx, reading.LocationID, reading.PM25, reading.Timestamp)
	if err != nil {
		log.Error("trend record failed", "error", err)
		return
	}

	subs, err := c.db.ActiveSubscribersForLocation(ctx, reading.LocationID)
	if err != nil {
		log.Error("fetch active subscribers failed", "error", err)
		return
	}

	locationName, err := c.db.LocationName(ctx, reading.LocationID)
	if err != nil {
		log.Error("fetch location name failed", "error", err)
		return
	}
	if locationName == "" {
		locationName = fmt.Sprintf("Lokasi #%d", reading.LocationID)
	}

	// Location-level representative score (general public, multiplier 1.0)
	// for the internal admin API (PRD section 12).
	general := riskscore.Compute(riskscore.Input{
		PM25: reading.PM25, PM10: reading.PM10,
		Temperature: reading.Temperature, Humidity: reading.Humidity,
		TrendAvgPM25: avg, TrendAvailable: avgAvailable, SensitivityLevel: 1,
	})
	if err := c.cache.SetLocation(ctx, redisstate.Entry{
		LocationID: reading.LocationID, Score: general.Score, Category: string(general.Category),
		Trend: string(general.Trend), PM25: reading.PM25, PM10: reading.PM10,
		Temp: reading.Temperature, Humidity: reading.Humidity, ComputedAt: reading.Timestamp,
	}); err != nil {
		log.Error("cache location score failed", "error", err)
	}

	if len(subs) == 0 {
		log.Debug("no active subscribers for location")
	}

	for _, sub := range subs {
		c.processSubscriber(ctx, log, reading, avg, avgAvailable, locationName, sub)
	}

	if err := c.rdb.XAck(ctx, c.rawStreamKey, c.consumerGroup, msg.ID).Err(); err != nil {
		log.Error("xack failed", "error", err)
		return
	}
	log.Debug("message acked")
}

func (c *Consumer) processSubscriber(ctx context.Context, log *slog.Logger, reading stream.RawReading, avg float64, avgAvailable bool, locationName string, sub db.Subscriber) {
	ulog := log.With("user_id", sub.UserID)

	result := riskscore.Compute(riskscore.Input{
		PM25: reading.PM25, PM10: reading.PM10,
		Temperature: reading.Temperature, Humidity: reading.Humidity,
		TrendAvgPM25: avg, TrendAvailable: avgAvailable, SensitivityLevel: sub.SensitivityLevel,
	})

	if err := c.db.InsertRiskScoreHistory(ctx, db.RiskScoreRecord{
		LocationID: reading.LocationID, UserID: sub.UserID,
		PM25: reading.PM25, PM10: reading.PM10,
		Temperature: reading.Temperature, Humidity: reading.Humidity,
		RiskScore: result.Score, Trend: string(result.Trend),
	}); err != nil {
		ulog.Error("insert risk_score_history failed", "error", err)
	}

	if err := c.cache.SetUser(ctx, sub.UserID, redisstate.Entry{
		LocationID: reading.LocationID, Score: result.Score, Category: string(result.Category),
		Trend: string(result.Trend), PM25: reading.PM25, PM10: reading.PM10,
		Temp: reading.Temperature, Humidity: reading.Humidity, ComputedAt: reading.Timestamp,
	}); err != nil {
		ulog.Error("cache user score failed", "error", err)
	}

	scoreStreamID, err := c.publisher.PublishScore(ctx, sub.UserID, reading.LocationID, result.Score, string(result.Trend))
	if err != nil {
		ulog.Error("publish risk-scores failed", "error", err)
	}

	shouldAlert, err := c.debounce.ShouldAlert(ctx, sub.UserID, result.Category)
	if err != nil {
		ulog.Error("debounce check failed", "error", err)
		shouldAlert = false
	}

	ulog.Info("score computed",
		"score", result.Score, "category", result.Category, "trend", result.Trend,
		"score_stream_id", scoreStreamID, "alert_worthy", result.Category.IsAlertWorthy(), "will_alert", shouldAlert,
	)

	if !shouldAlert {
		return
	}

	message := riskscore.FormatAlertMessage(riskscore.MessageInput{
		LocationName: locationName, Score: result.Score, Category: result.Category,
		PM25: reading.PM25, PM25Previous: avg, PM25Available: avgAvailable,
		Temperature: reading.Temperature, Humidity: reading.Humidity,
		ConditionType: sub.ConditionType, SensitivityLevel: sub.SensitivityLevel,
	})

	// alert_history is written by telegram-notifier-service after it
	// actually sends the message (PRD section 6 diagram) — not here at
	// publish time, to avoid a duplicate row if delivery later fails.
	alertStreamID, err := c.publisher.PublishAlert(ctx, sub.UserID, reading.LocationID, result.Score, message)
	if err != nil {
		ulog.Error("publish risk-alerts failed", "error", err)
		return
	}

	if err := c.debounce.MarkSent(ctx, sub.UserID, result.Category); err != nil {
		ulog.Error("mark debounce sent failed", "error", err)
	}

	ulog.Warn("alert published", "alert_stream_id", alertStreamID, "category", result.Category)
}
