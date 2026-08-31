// Package db wraps the PostgreSQL connection pool (pgx) and every query the
// risk engine needs: resolving active subscribers for a location and
// persisting risk_score_history (PRD section 8).
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

// Migrate applies schema.sql. It's idempotent (CREATE TABLE/INDEX IF NOT
// EXISTS), so running it on every startup is safe and needs no separate
// migration tool.
func (d *DB) Migrate(ctx context.Context) error {
	_, err := d.Pool.Exec(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}

// Subscriber is one active user monitoring a location, with the
// sensitivity profile the risk score is personalized against.
type Subscriber struct {
	UserID            int64
	TelegramChatID    int64
	ConditionType     string
	SensitivityLevel  int16
}

// ActiveSubscribersForLocation returns every user with an active
// subscription to locationID, along with their latest sensitivity profile
// (PRD section 10: "join dgn subscription (Postgres)").
func (d *DB) ActiveSubscribersForLocation(ctx context.Context, locationID int64) ([]Subscriber, error) {
	const query = `
		SELECT
			u.id,
			u.telegram_chat_id,
			COALESCE(sp.condition_type, 'umum') AS condition_type,
			COALESCE(sp.sensitivity_level, 1) AS sensitivity_level
		FROM user_subscriptions us
		JOIN users u ON u.id = us.user_id
		LEFT JOIN LATERAL (
			SELECT condition_type, sensitivity_level
			FROM sensitivity_profiles
			WHERE user_id = u.id
			ORDER BY updated_at DESC
			LIMIT 1
		) sp ON true
		WHERE us.location_id = $1 AND us.is_active = true
	`

	rows, err := d.Pool.Query(ctx, query, locationID)
	if err != nil {
		return nil, fmt.Errorf("query active subscribers: %w", err)
	}
	defer rows.Close()

	var subs []Subscriber
	for rows.Next() {
		var s Subscriber
		if err := rows.Scan(&s.UserID, &s.TelegramChatID, &s.ConditionType, &s.SensitivityLevel); err != nil {
			return nil, fmt.Errorf("scan subscriber row: %w", err)
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriber rows: %w", err)
	}
	return subs, nil
}

// LocationName returns city_name for a location, used in alert messages.
// Returns ("", nil) if the location doesn't exist (caller decides fallback).
func (d *DB) LocationName(ctx context.Context, locationID int64) (string, error) {
	var name string
	err := d.Pool.QueryRow(ctx, `SELECT city_name FROM locations WHERE id = $1`, locationID).Scan(&name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("query location name: %w", err)
	}
	return name, nil
}

// RiskScoreRecord is one row to persist into risk_score_history.
type RiskScoreRecord struct {
	LocationID  int64
	UserID      int64
	PM25        float64
	PM10        float64
	Temperature float64
	Humidity    float64
	RiskScore   float64
	Trend       string
}

// InsertRiskScoreHistory persists one computed score (PRD section 8/10:
// every score is recorded, whether or not it crosses the alert threshold).
func (d *DB) InsertRiskScoreHistory(ctx context.Context, r RiskScoreRecord) error {
	const query = `
		INSERT INTO risk_score_history
			(location_id, user_id, pm25, pm10, temperature, humidity, risk_score, trend, computed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
	`
	_, err := d.Pool.Exec(ctx, query,
		r.LocationID, r.UserID, r.PM25, r.PM10, r.Temperature, r.Humidity, r.RiskScore, r.Trend,
	)
	if err != nil {
		return fmt.Errorf("insert risk_score_history: %w", err)
	}
	return nil
}
