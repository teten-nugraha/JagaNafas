// Package consumer runs the Redis Streams consumer group loop for
// stream:risk-alerts (PRD section 6 & 9): resolve the user's Telegram chat,
// send via Telegram Bot API, record alert_history, and XACK — with
// periodic XAUTOCLAIM so a crash before ACK doesn't lose messages (PRD
// section 13 idempotency).
package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jaganapas/telegram-notifier-service/internal/config"
	"github.com/jaganapas/telegram-notifier-service/internal/db"
	"github.com/jaganapas/telegram-notifier-service/internal/stream"
	"github.com/jaganapas/telegram-notifier-service/internal/telegram"
)

type Consumer struct {
	rdb *redis.Client
	db  *db.DB
	tg  *telegram.Client
	log *slog.Logger

	streamKey     string
	consumerGroup string
	consumerName  string
	readBlock     time.Duration
	readCount     int64
	claimInterval time.Duration
	claimMinIdle  time.Duration
}

func New(cfg *config.Config, rdb *redis.Client, database *db.DB, tg *telegram.Client, log *slog.Logger) *Consumer {
	return &Consumer{
		rdb:           rdb,
		db:            database,
		tg:            tg,
		log:           log,
		streamKey:     cfg.AlertsStreamKey,
		consumerGroup: cfg.ConsumerGroup,
		consumerName:  cfg.ConsumerName,
		readBlock:     cfg.ReadBlock,
		readCount:     cfg.ReadCount,
		claimInterval: cfg.ClaimInterval,
		claimMinIdle:  cfg.ClaimMinIdle,
	}
}

// EnsureGroup creates the consumer group (and the stream, if it doesn't
// exist yet) starting from the beginning. Safe every startup — BUSYGROUP
// just means it already exists.
func (c *Consumer) EnsureGroup(ctx context.Context) error {
	err := c.rdb.XGroupCreateMkStream(ctx, c.streamKey, c.consumerGroup, "0").Err()
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
	c.log.Info("consumer read loop starting", "stream", c.streamKey, "group", c.consumerGroup, "consumer", c.consumerName)

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
			Streams:  []string{c.streamKey, ">"},
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
// consumer crashed mid-send — and reprocesses them.
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
			Stream:   c.streamKey,
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

// handleMessage resolves the chat id, sends via Telegram, records
// alert_history on success, and ACKs. Malformed messages and permanent
// send failures (bot blocked, unknown user/chat) are ACKed and dropped —
// retrying them can never succeed. Transient failures (Postgres/Telegram
// temporarily unreachable, rate limits) skip the ACK so XAUTOCLAIM retries.
func (c *Consumer) handleMessage(ctx context.Context, msg redis.XMessage) {
	event, err := stream.ParseAlertEvent(msg.Values)
	if err != nil {
		c.log.Error("malformed alert event, dropping", "message_id", msg.ID, "values", msg.Values, "error", err)
		c.ack(ctx, msg.ID)
		return
	}

	log := c.log.With("message_id", msg.ID, "user_id", event.UserID, "location_id", event.LocationID)

	chatID, err := c.db.TelegramChatID(ctx, event.UserID)
	if err != nil {
		if errors.Is(err, db.ErrUserNotFound) {
			log.Error("user not found, dropping alert", "error", err)
			c.ack(ctx, msg.ID)
			return
		}
		log.Error("lookup telegram_chat_id failed, will retry", "error", err)
		return
	}

	sendStart := time.Now()
	err = c.tg.SendMessage(ctx, chatID, event.Message)
	sendElapsed := time.Since(sendStart)

	if err != nil {
		var sendErr *telegram.SendError
		if errors.As(err, &sendErr) {
			if sendErr.Permanent {
				log.Warn("permanent telegram send failure, dropping",
					"chat_id", chatID, "error_code", sendErr.ErrorCode, "description", sendErr.Description,
					"elapsed_ms", sendElapsed.Milliseconds(),
				)
				c.ack(ctx, msg.ID)
				return
			}
			log.Error("transient telegram send failure, will retry",
				"chat_id", chatID, "error_code", sendErr.ErrorCode, "description", sendErr.Description,
				"elapsed_ms", sendElapsed.Milliseconds(),
			)
			return
		}
		log.Error("telegram send failed, will retry", "chat_id", chatID, "elapsed_ms", sendElapsed.Milliseconds(), "error", err)
		return
	}

	log.Info("alert sent", "chat_id", chatID, "score", event.Score, "elapsed_ms", sendElapsed.Milliseconds())

	if err := c.db.InsertAlertHistory(ctx, event.UserID, event.LocationID, event.Score, event.Message); err != nil {
		log.Error("insert alert_history failed", "error", err)
		// The message was already delivered — acking anyway avoids sending
		// a duplicate Telegram message on retry; the audit row is best-effort.
	}

	c.ack(ctx, msg.ID)
}

func (c *Consumer) ack(ctx context.Context, messageID string) {
	if err := c.rdb.XAck(ctx, c.streamKey, c.consumerGroup, messageID).Err(); err != nil {
		c.log.Error("xack failed", "message_id", messageID, "error", err)
	}
}
