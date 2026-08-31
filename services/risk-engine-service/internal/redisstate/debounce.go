package redisstate

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jaganapas/risk-engine-service/internal/riskscore"
)

type DebounceStore struct {
	rdb    *redis.Client
	window time.Duration
}

func NewDebounceStore(rdb *redis.Client, window time.Duration) *DebounceStore {
	return &DebounceStore{rdb: rdb, window: window}
}

func debounceKey(userID int64) string {
	return fmt.Sprintf("alert:lastsent:%d", userID)
}

// ShouldAlert implements the PRD section 10 alert rule: send if the score is
// Berisiko/Bahaya AND (no alert in the debounce window OR the category
// jumped >=2 levels since the last alert — emergency override).
func (d *DebounceStore) ShouldAlert(ctx context.Context, userID int64, category riskscore.Category) (bool, error) {
	if !category.IsAlertWorthy() {
		return false, nil
	}

	val, err := d.rdb.Get(ctx, debounceKey(userID)).Result()
	if err == redis.Nil {
		return true, nil // no alert on record within the window
	}
	if err != nil {
		return false, fmt.Errorf("get debounce state: %w", err)
	}

	lastCategory, ok := parseLastCategory(val)
	if !ok {
		// Corrupt/unexpected value: fail safe by allowing the alert rather
		// than silently going quiet.
		return true, nil
	}

	if category.Level()-lastCategory.Level() >= 2 {
		return true, nil // emergency override
	}
	return false, nil
}

// MarkSent records that an alert was just sent, starting a fresh debounce
// window and remembering the category for the next override check.
func (d *DebounceStore) MarkSent(ctx context.Context, userID int64, category riskscore.Category) error {
	val := strconv.Itoa(category.Level())
	if err := d.rdb.Set(ctx, debounceKey(userID), val, d.window).Err(); err != nil {
		return fmt.Errorf("set debounce state: %w", err)
	}
	return nil
}

func parseLastCategory(val string) (riskscore.Category, bool) {
	level, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		return riskscore.CategoryAman, false
	}
	cat, ok := riskscore.CategoryFromLevel(level)
	return cat, ok
}
