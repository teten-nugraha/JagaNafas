// Package redisstate holds bot-service's Redis-backed state: the per-chat
// onboarding/edit conversation (FSM), and reads of the score cache
// risk-engine-service writes (for /status).
package redisstate

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Step is where a chat currently is in a multi-message flow (PRD section 5:
// /start onboarding, or the location/profile-only variants used by
// /ubahlokasi and /ubahprofil).
type Step string

const (
	StepNone                Step = ""
	StepAwaitingLocation    Step = "awaiting_location"
	StepAwaitingCondition   Step = "awaiting_condition"
	StepAwaitingSensitivity Step = "awaiting_sensitivity"
)

// Session is the in-progress state for one chat. Purely a local record of
// "what has this chat entered so far in the current flow" — the durable
// result (location, profile, subscription) is written to Postgres only
// once the flow completes.
type Session struct {
	Step          Step
	LocationOnly  bool   // true for /ubahlokasi: skip condition/sensitivity after location
	ProfileOnly   bool   // true for /ubahprofil: started at condition, skip location
	LocationID    int64  // set once a location has been resolved this flow
	ConditionType string // set once a condition button has been pressed
}

type SessionStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewSessionStore(rdb *redis.Client, ttl time.Duration) *SessionStore {
	return &SessionStore{rdb: rdb, ttl: ttl}
}

func sessionKey(chatID int64) string {
	return fmt.Sprintf("bot:session:%d", chatID)
}

func (s *SessionStore) Get(ctx context.Context, chatID int64) (Session, error) {
	vals, err := s.rdb.HGetAll(ctx, sessionKey(chatID)).Result()
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	if len(vals) == 0 {
		return Session{Step: StepNone}, nil
	}

	sess := Session{Step: Step(vals["step"])}
	sess.LocationOnly = vals["location_only"] == "1"
	sess.ProfileOnly = vals["profile_only"] == "1"
	sess.ConditionType = vals["condition_type"]
	if vals["location_id"] != "" {
		fmt.Sscanf(vals["location_id"], "%d", &sess.LocationID)
	}
	return sess, nil
}

func (s *SessionStore) Save(ctx context.Context, chatID int64, sess Session) error {
	key := sessionKey(chatID)
	fields := map[string]any{
		"step":           string(sess.Step),
		"location_only":  boolField(sess.LocationOnly),
		"profile_only":   boolField(sess.ProfileOnly),
		"location_id":    sess.LocationID,
		"condition_type": sess.ConditionType,
	}
	if err := s.rdb.HSet(ctx, key, fields).Err(); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return s.rdb.Expire(ctx, key, s.ttl).Err()
}

func (s *SessionStore) Clear(ctx context.Context, chatID int64) error {
	if err := s.rdb.Del(ctx, sessionKey(chatID)).Err(); err != nil {
		return fmt.Errorf("clear session: %w", err)
	}
	return nil
}

func boolField(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
