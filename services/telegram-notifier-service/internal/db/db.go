// Package db wraps the PostgreSQL connection pool (pgx): resolving a user's
// telegram_chat_id to send to, and recording alert_history once a message
// is actually delivered (PRD section 6 & 8).
package db

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

type DB struct {
	Pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &DB{Pool: pool}, nil
}

func (d *DB) Close() {
	d.Pool.Close()
}

// Migrate applies schema.sql (CREATE TABLE/INDEX IF NOT EXISTS — idempotent,
// same schema every JagaNapas service embeds, so whichever starts first
// creates it and the rest are no-ops).
func (d *DB) Migrate(ctx context.Context) error {
	_, err := d.Pool.Exec(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

// ErrUserNotFound means the alert references a userID with no matching row
// — not a transient failure, so callers should not retry it.
var ErrUserNotFound = errors.New("user not found")

// TelegramChatID resolves a user's chat id to send the Telegram message to.
func (d *DB) TelegramChatID(ctx context.Context, userID int64) (int64, error) {
	var chatID int64
	err := d.Pool.QueryRow(ctx, `SELECT telegram_chat_id FROM users WHERE id = $1`, userID).Scan(&chatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrUserNotFound
		}
		return 0, fmt.Errorf("query telegram_chat_id: %w", err)
	}
	return chatID, nil
}

// InsertAlertHistory records an alert that was actually sent (PRD section 6
// diagram: notifier writes alert_history after Telegram sendMessage
// succeeds — risk-engine only publishes to the stream).
func (d *DB) InsertAlertHistory(ctx context.Context, userID, locationID int64, riskScore float64, message string) error {
	const query = `
		INSERT INTO alert_history (user_id, location_id, risk_score, message, sent_at)
		VALUES ($1, $2, $3, $4, now())
	`
	_, err := d.Pool.Exec(ctx, query, userID, locationID, riskScore, message)
	if err != nil {
		return fmt.Errorf("insert alert_history: %w", err)
	}
	return nil
}
