// Package riskscore implements the personalized risk score calculation from
// PRD section 10. The PRD specifies the pipeline (normalize → discomfort
// index → trend weight → sensitivity multiplier → 0-100 categorized score)
// but not exact breakpoints/formulas, so each step below documents the
// concrete choice made and why.
package riskscore

import "math"

// Trend is the human-readable direction used in messages and stream payloads.
type Trend string

const (
	TrendNaik   Trend = "naik"
	TrendTurun  Trend = "turun"
	TrendStabil Trend = "stabil"
)

// Input is everything Compute needs for one user-location score.
type Input struct {
	PM25             float64
	PM10             float64
	Temperature      float64
	Humidity         float64
	TrendAvgPM25     float64 // rolling 3h average; ignored if TrendAvailable is false
	TrendAvailable   bool
	SensitivityLevel int16 // 1-5, from sensitivity_profiles
}

// Result is the computed score plus the intermediate values worth logging /
// persisting (trend label goes straight into risk_score_history.trend).
type Result struct {
	Score    float64
	Category Category
	Trend    Trend
}

// Compute runs the full PRD section 10 pipeline for one subscriber.
func Compute(in Input) Result {
	pollution := normalizePollution(in.PM25, in.PM10)
	comfort := discomfortAdjustment(in.Temperature, in.Humidity)
	trend, trendWeight := trendAndWeight(in.PM25, in.TrendAvgPM25, in.TrendAvailable)
	multiplier := sensitivityMultiplier(in.SensitivityLevel)

	score := (pollution + comfort) * trendWeight * multiplier
	score = clamp(score, 0, 100)

	return Result{
		Score:    score,
		Category: Categorize(score),
		Trend:    trend,
	}
}

// normalizePollution maps PM2.5/PM10 (µg/m³) to a 0-100 scale using
// piecewise-linear anchors derived from WHO 24h guideline values and typical
// Indonesian ISPU "Tidak Sehat"/"Berbahaya" breakpoints (PRD step 1: "skala
// WHO/BMKG"), then takes the worse of the two pollutants — the standard AQI
// convention of letting the dominant pollutant drive the index.
func normalizePollution(pm25, pm10 float64) float64 {
	return math.Max(
		interpolate(pm25, pm25Breakpoints),
		interpolate(pm10, pm10Breakpoints),
	)
}

type breakpoint struct{ conc, score float64 }

var pm25Breakpoints = []breakpoint{
	{0, 0}, {15, 30}, {55, 60}, {150, 80}, {250, 100},
}

var pm10Breakpoints = []breakpoint{
	{0, 0}, {50, 30}, {150, 60}, {350, 80}, {420, 100},
}

func interpolate(x float64, bp []breakpoint) float64 {
	if x <= bp[0].conc {
		return bp[0].score
	}
	last := bp[len(bp)-1]
	if x >= last.conc {
		return last.score
	}
	for i := 1; i < len(bp); i++ {
		if x <= bp[i].conc {
			lo, hi := bp[i-1], bp[i]
			frac := (x - lo.conc) / (hi.conc - lo.conc)
			return lo.score + frac*(hi.score-lo.score)
		}
	}
	return last.score
}

// discomfortAdjustment is a simplified heat-index-style discomfort score
// (Thom's discomfort index) turned into a small additive nudge on the final
// score (PRD step 2), capped so weather never dominates a pollution-driven
// score — air quality is the primary signal per PRD section 1.
func discomfortAdjustment(tempC, humidityPct float64) float64 {
	di := tempC - 0.55*(1-humidityPct/100)*(tempC-14.5)
	// DI ~24-27 is "mildly uncomfortable" for most people, >27 "distressing".
	adj := (di - 24) * 2
	return clamp(adj, -5, 10)
}

// trendAndWeight compares the current PM2.5 reading against the rolling 3h
// average (PRD step 3): a fast rise is weighted higher, a fall is weighted
// slightly lower, both clamped to a modest ±20% so trend nudges rather than
// dominates the score.
func trendAndWeight(current, avg float64, avgAvailable bool) (Trend, float64) {
	if !avgAvailable || avg <= 0 {
		return TrendStabil, 1.0
	}

	pctChange := (current - avg) / avg
	switch {
	case pctChange >= 0.20:
		return TrendNaik, clamp(1.0+pctChange, 1.0, 1.2)
	case pctChange <= -0.20:
		return TrendTurun, clamp(1.0+pctChange, 0.9, 1.0)
	default:
		return TrendStabil, 1.0
	}
}

// sensitivityMultiplier linearly maps sensitivity_level 1-5 to 1.0-1.8 (PRD
// step 4: "1.0 untuk umum, sampai 1.8 untuk asma berat").
func sensitivityMultiplier(level int16) float64 {
	if level < 1 {
		level = 1
	}
	if level > 5 {
		level = 5
	}
	return 1.0 + float64(level-1)*0.2
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
