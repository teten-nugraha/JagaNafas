package bot

import "github.com/jaganapas/bot-service/internal/telegram"

// conditionKeyboard mirrors PRD section 5 step 4's options exactly.
func conditionKeyboard() telegram.InlineKeyboardMarkup {
	return telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "Tidak ada kondisi khusus", CallbackData: "cond:umum"}},
		{{Text: "Asma ringan", CallbackData: "cond:asma_ringan"}},
		{{Text: "Asma sedang-berat", CallbackData: "cond:asma_berat"}},
		{{Text: "ISPA berulang", CallbackData: "cond:ispa_berulang"}},
		{{Text: "Lainnya", CallbackData: "cond:lainnya"}},
	}}
}

// sensitivityKeyboard offers the 1-5 scale PRD section 5 step 5 describes,
// with Rendah/Sedang/Tinggi anchors at the ends and middle.
func sensitivityKeyboard() telegram.InlineKeyboardMarkup {
	return telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "1 (Rendah)", CallbackData: "sens:1"}, {Text: "2", CallbackData: "sens:2"}},
		{{Text: "3 (Sedang)", CallbackData: "sens:3"}, {Text: "4", CallbackData: "sens:4"}},
		{{Text: "5 (Tinggi)", CallbackData: "sens:5"}},
	}}
}

// locationRequestKeyboard offers a one-tap "share my location" button (PRD
// section 5 step 2's second option), alongside typing a city name.
func locationRequestKeyboard() telegram.ReplyKeyboardMarkup {
	return telegram.ReplyKeyboardMarkup{
		Keyboard: [][]telegram.KeyboardButton{
			{{Text: "📍 Kirim Lokasi Saya", RequestLocation: true}},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: true,
	}
}
