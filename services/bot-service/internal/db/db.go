// Package db wraps the PostgreSQL connection pool (pgx) and every query
// bot-service needs: onboarding a user, resolving/storing their location
// and sensitivity profile, managing their subscription, and reading their
// history (PRD section 5 & 8).
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
// same schema every JagaNapas service embeds).
func (d *DB) Migrate(ctx context.Context) error {
	_, err := d.Pool.Exec(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

// UpsertUser ensures a users row exists for this Telegram chat, keeping the
// username in sync, and returns its id.
func (d *DB) UpsertUser(ctx context.Context, telegramChatID int64, username string) (int64, error) {
	const query = `
		INSERT INTO users (telegram_chat_id, username)
		VALUES ($1, $2)
		ON CONFLICT (telegram_chat_id) DO UPDATE SET username = EXCLUDED.username
		RETURNING id
	`
	var id int64
	if err := d.Pool.QueryRow(ctx, query, telegramChatID, nullable(username)).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert user: %w", err)
	}
	return id, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// UserIDByChatID resolves a user's internal id from their Telegram chat id.
// Returns ErrNotFound if they've never run /start.
func (d *DB) UserIDByChatID(ctx context.Context, telegramChatID int64) (int64, error) {
	var id int64
	err := d.Pool.QueryRow(ctx, `SELECT id FROM users WHERE telegram_chat_id = $1`, telegramChatID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("query user by chat id: %w", err)
	}
	return id, nil
}

// ErrNotFound is returned by lookups with no matching row.
var ErrNotFound = errors.New("not found")

// UpsertLocation finds or creates a location by (lat, lon) — the schema's
// UNIQUE(lat, lon) constraint is the dedupe key (PRD section 8) — updating
// the display name if it changed (e.g. richer name from a later geocode).
func (d *DB) UpsertLocation(ctx context.Context, cityName string, lat, lon float64) (int64, error) {
	const query = `
		INSERT INTO locations (city_name, lat, lon)
		VALUES ($1, $2, $3)
		ON CONFLICT (lat, lon) DO UPDATE SET city_name = EXCLUDED.city_name
		RETURNING id
	`
	var id int64
	if err := d.Pool.QueryRow(ctx, query, cityName, lat, lon).Scan(&id); err != nil {
		return 0, fmt.Errorf("upsert location: %w", err)
	}
	return id, nil
}

func (d *DB) LocationName(ctx context.Context, locationID int64) (string, error) {
	var name string
	err := d.Pool.QueryRow(ctx, `SELECT city_name FROM locations WHERE id = $1`, locationID).Scan(&name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("query location name: %w", err)
	}
	return name, nil
}

// InsertSensitivityProfile records a new profile snapshot. Deliberately an
// INSERT, not an UPDATE: every service that reads sensitivity picks the
// most recent row (ORDER BY updated_at DESC LIMIT 1), so history of past
// profiles is kept for free.
func (d *DB) InsertSensitivityProfile(ctx context.Context, userID int64, conditionType string, sensitivityLevel int16) error {
	const query = `
		INSERT INTO sensitivity_profiles (user_id, condition_type, sensitivity_level, updated_at)
		VALUES ($1, $2, $3, now())
	`
	_, err := d.Pool.Exec(ctx, query, userID, conditionType, sensitivityLevel)
	if err != nil {
		return fmt.Errorf("insert sensitivity profile: %w", err)
	}
	return nil
}

func (d *DB) LatestSensitivityProfile(ctx context.Context, userID int64) (conditionType string, level int16, err error) {
	const query = `
		SELECT condition_type, sensitivity_level
		FROM sensitivity_profiles
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT 1
	`
	err = d.Pool.QueryRow(ctx, query, userID).Scan(&conditionType, &level)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", 0, ErrNotFound
		}
		return "", 0, fmt.Errorf("query latest sensitivity profile: %w", err)
	}
	return conditionType, level, nil
}

// ActiveSubscription returns the location a user currently monitors. v1
// supports a single active location per user (PRD section 16 leaves
// multi-location as an open question) — /ubahlokasi replaces it rather
// than adding a second one.
func (d *DB) ActiveSubscription(ctx context.Context, userID int64) (locationID int64, err error) {
	const query = `
		SELECT location_id FROM user_subscriptions
		WHERE user_id = $1 AND is_active = true
		ORDER BY created_at DESC
		LIMIT 1
	`
	err = d.Pool.QueryRow(ctx, query, userID).Scan(&locationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("query active subscription: %w", err)
	}
	return locationID, nil
}

// ReplaceSubscription deactivates any existing active subscription(s) for
// the user and activates locationID as the new one, in a single
// transaction so a crash mid-way never leaves the user with zero or two
// active subscriptions.
func (d *DB) ReplaceSubscription(ctx context.Context, userID, locationID int64) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE user_subscriptions SET is_active = false WHERE user_id = $1 AND is_active = true`, userID); err != nil {
		return fmt.Errorf("deactivate existing subscriptions: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_subscriptions (user_id, location_id, is_active) VALUES ($1, $2, true)`, userID, locationID); err != nil {
		return fmt.Errorf("insert subscription: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Unsubscribe deactivates every active subscription for the user (PRD
// section 5: /berhenti).
func (d *DB) Unsubscribe(ctx context.Context, userID int64) error {
	_, err := d.Pool.Exec(ctx, `UPDATE user_subscriptions SET is_active = false WHERE user_id = $1 AND is_active = true`, userID)
	if err != nil {
		return fmt.Errorf("deactivate subscriptions: %w", err)
	}
	return nil
}

// HistoryRow is one risk_score_history entry for /riwayat.
type HistoryRow struct {
	Score      float64
	Trend      string
	ComputedAt time.Time
}

// RecentHistory returns a user's most recent risk score entries within the
// last `since`..now window, newest first (PRD section 5: /riwayat).
func (d *DB) RecentHistory(ctx context.Context, userID int64, since time.Time, limit int) ([]HistoryRow, error) {
	const query = `
		SELECT risk_score, trend, computed_at
		FROM risk_score_history
		WHERE user_id = $1 AND computed_at >= $2
		ORDER BY computed_at DESC
		LIMIT $3
	`
	rows, err := d.Pool.Query(ctx, query, userID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent history: %w", err)
	}
	defer rows.Close()

	var out []HistoryRow
	for rows.Next() {
		var h HistoryRow
		if err := rows.Scan(&h.Score, &h.Trend, &h.ComputedAt); err != nil {
			return nil, fmt.Errorf("scan history row: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history rows: %w", err)
	}
	return out, nil
}
