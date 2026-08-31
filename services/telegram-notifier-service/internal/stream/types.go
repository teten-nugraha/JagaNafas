// Package stream parses messages off stream:risk-alerts (PRD section 9).
package stream

import (
	"fmt"
	"strconv"
	"time"
)

// AlertEvent is one parsed message from stream:risk-alerts.
type AlertEvent struct {
	UserID     int64
	LocationID int64
	Score      float64
	Message    string
	Timestamp  time.Time
}

// ParseAlertEvent decodes the string-typed field map XReadGroup returns
// (fields were written by risk-engine-service) into an AlertEvent, failing
// loudly on any malformed field so a bad message can be logged with enough
// detail to fix at the source rather than silently dropped.
func ParseAlertEvent(values map[string]any) (AlertEvent, error) {
	userID, err := parseInt(values, "userId")
	if err != nil {
		return AlertEvent{}, err
	}
	locationID, err := parseInt(values, "locationId")
	if err != nil {
		return AlertEvent{}, err
	}
	score, err := parseFloat(values, "score")
	if err != nil {
		return AlertEvent{}, err
	}
	message, err := parseString(values, "message")
	if err != nil {
		return AlertEvent{}, err
	}

	ts := time.Now().UTC()
	if raw, ok := values["timestamp"]; ok {
		if s, ok := raw.(string); ok {
			if parsed, err := time.Parse(time.RFC3339, s); err == nil {
				ts = parsed
			}
		}
	}

	return AlertEvent{
		UserID:     userID,
		LocationID: locationID,
		Score:      score,
		Message:    message,
		Timestamp:  ts,
	}, nil
}

func parseString(values map[string]any, field string) (string, error) {
	raw, ok := values[field]
	if !ok {
		return "", fmt.Errorf("missing field %q", field)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("field %q is not a string: %v", field, raw)
	}
	return s, nil
}

func parseFloat(values map[string]any, field string) (float64, error) {
	s, err := parseString(values, field)
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("field %q not a float: %w", field, err)
	}
	return f, nil
}

func parseInt(values map[string]any, field string) (int64, error) {
	s, err := parseString(values, field)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("field %q not an int: %w", field, err)
	}
	return n, nil
}
