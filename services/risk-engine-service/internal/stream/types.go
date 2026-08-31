// Package stream handles the Redis Streams side of the risk engine:
// parsing raw-environment-data messages and publishing risk-scores /
// risk-alerts (PRD section 9).
package stream

import (
	"fmt"
	"strconv"
	"time"
)

// RawReading is one parsed message from stream:raw-environment-data.
type RawReading struct {
	LocationID  int64
	PM25        float64
	PM10        float64
	Temperature float64
	Humidity    float64
	Timestamp   time.Time
}

// ParseRawReading decodes the string-typed field map XReadGroup returns
// (fields were written by scheduler-service as decimal strings) into a
// RawReading, failing loudly on any malformed field so a bad message can be
// logged with enough detail to fix at the source.
func ParseRawReading(values map[string]any) (RawReading, error) {
	locationID, err := parseInt(values, "locationId")
	if err != nil {
		return RawReading{}, err
	}
	pm25, err := parseFloat(values, "pm25")
	if err != nil {
		return RawReading{}, err
	}
	pm10, err := parseFloat(values, "pm10")
	if err != nil {
		return RawReading{}, err
	}
	temp, err := parseFloat(values, "temp")
	if err != nil {
		return RawReading{}, err
	}
	humidity, err := parseFloat(values, "humidity")
	if err != nil {
		return RawReading{}, err
	}

	ts := time.Now().UTC()
	if raw, ok := values["timestamp"]; ok {
		if s, ok := raw.(string); ok {
			if parsed, err := time.Parse(time.RFC3339, s); err == nil {
				ts = parsed
			}
		}
	}

	return RawReading{
		LocationID:  locationID,
		PM25:        pm25,
		PM10:        pm10,
		Temperature: temp,
		Humidity:    humidity,
		Timestamp:   ts,
	}, nil
}

func parseFloat(values map[string]any, field string) (float64, error) {
	raw, ok := values[field]
	if !ok {
		return 0, fmt.Errorf("missing field %q", field)
	}
	s, ok := raw.(string)
	if !ok {
		return 0, fmt.Errorf("field %q is not a string: %v", field, raw)
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("field %q not a float: %w", field, err)
	}
	return f, nil
}

func parseInt(values map[string]any, field string) (int64, error) {
	raw, ok := values[field]
	if !ok {
		return 0, fmt.Errorf("missing field %q", field)
	}
	s, ok := raw.(string)
	if !ok {
		return 0, fmt.Errorf("field %q is not a string: %v", field, raw)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("field %q not an int: %w", field, err)
	}
	return n, nil
}
