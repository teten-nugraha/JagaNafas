package bot

import (
	"fmt"
	"strings"
	"time"

	"github.com/jaganapas/bot-service/internal/db"
	"github.com/jaganapas/bot-service/internal/redisstate"
)

// categoryLabel mirrors risk-engine-service's category bands (PRD section
// 10 step 5) — duplicated here as a small, PRD-defined constant rather than
// a shared module, consistent with how each service embeds its own copy of
// schema.sql.
func categoryLabel(score float64) string {
	switch {
	case score >= 81:
		return "Bahaya"
	case score >= 61:
		return "Berisiko"
	case score >= 31:
		return "Waspada"
	default:
		return "Aman"
	}
}

func categoryIndicator(category string) string {
	switch category {
	case "Bahaya":
		return "🔴"
	case "Berisiko":
		return "🟠"
	case "Waspada":
		return "🟡"
	default:
		return "🟢"
	}
}

func formatStatus(locationName string, c redisstate.CachedScore) string {
	category := c.Category
	if category == "" {
		category = categoryLabel(c.Score)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s Status Kualitas Udara\n\n", categoryIndicator(category))
	fmt.Fprintf(&sb, "Lokasi: %s\n", locationName)
	fmt.Fprintf(&sb, "Skor Risiko: %.0f/100 (%s)\n", c.Score, category)
	fmt.Fprintf(&sb, "PM2.5: %.0f µg/m³\n", c.PM25)
	fmt.Fprintf(&sb, "Trend: %s\n", c.Trend)
	sb.WriteString("\nKetik /riwayat untuk lihat 7 hari terakhir.")
	return sb.String()
}

func formatHistory(rows []db.HistoryRow, days int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "📈 Riwayat %d Hari Terakhir\n\n", days)
	for _, r := range rows {
		fmt.Fprintf(&sb, "%s %s — %.0f/100 (%s, %s)\n",
			categoryIndicator(categoryLabel(r.Score)),
			r.ComputedAt.In(time.Local).Format("02 Jan 15:04"),
			r.Score, categoryLabel(r.Score), r.Trend,
		)
	}
	return sb.String()
}
