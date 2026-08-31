package riskscore

import "fmt"

// MessageInput carries everything FormatAlertMessage needs to render the
// PRD section 11 notification template.
type MessageInput struct {
	LocationName     string
	Score            float64
	Category         Category
	PM25             float64
	PM25Previous     float64
	PM25Available    bool // false when there was no rolling-window average yet
	Temperature      float64
	Humidity         float64
	ConditionType    string
	SensitivityLevel int16
}

// FormatAlertMessage renders the Telegram alert text following the PRD
// section 11 example format, personalized with the user's condition.
func FormatAlertMessage(in MessageInput) string {
	pm25Trend := ""
	if in.PM25Available {
		pm25Trend = fmt.Sprintf(" (naik dari %.0f µg/m³, 3 jam terakhir)", in.PM25Previous)
	}

	advice := adviceFor(in.Category, in.ConditionType)

	return fmt.Sprintf(
		"⚠️ JagaNapas — Peringatan Kualitas Udara\n\n"+
			"Lokasi: %s\n"+
			"Skor Risiko: %.0f/100 (%s)\n"+
			"PM2.5: %.0f µg/m³%s\n"+
			"Suhu: %.0f°C, Kelembapan: %.0f%%\n\n"+
			"%s\n\n"+
			"Ketik /status untuk cek kondisi terkini.",
		in.LocationName,
		in.Score, in.Category,
		in.PM25, pm25Trend,
		in.Temperature, in.Humidity,
		advice,
	)
}

func adviceFor(category Category, conditionType string) string {
	condition := conditionLabel(conditionType)

	intro := fmt.Sprintf("Karena kamu tercatat memiliki riwayat %s,\ndisarankan untuk:", condition)
	if conditionType == "" || conditionType == "umum" {
		intro = "Disarankan untuk:"
	}

	tips := "• Gunakan masker N95 jika keluar rumah\n• Hindari aktivitas fisik berat di luar ruangan"
	if category == CategoryBahaya {
		tips += "\n• Pastikan obat pereda gejala tersedia\n• Pertimbangkan untuk tetap di dalam rumah"
	} else {
		tips += "\n• Pastikan obat pereda gejala tersedia"
	}

	return intro + "\n" + tips
}

func conditionLabel(conditionType string) string {
	switch conditionType {
	case "asma_ringan":
		return "asma ringan"
	case "asma_berat":
		return "asma sedang-berat"
	case "ispa_berulang":
		return "ISPA berulang"
	default:
		return "kondisi pernapasan sensitif"
	}
}
