package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jaganapas/bot-service/internal/openmeteo"
	"github.com/jaganapas/bot-service/internal/redisstate"
	"github.com/jaganapas/bot-service/internal/telegram"
)

// handleLocationInput resolves either a shared Telegram location or a typed
// city name into coordinates, then either finishes /ubahlokasi (LocationOnly)
// or advances a fresh /start flow to the condition step (PRD section 5).
func (b *Bot) handleLocationInput(ctx context.Context, chatID int64, msg *telegram.Message, sess redisstate.Session) {
	var name string
	var lat, lon float64

	switch {
	case msg.Location != nil:
		// No reverse-geocoding service in the stack (Open-Meteo only does
		// forward geocoding) — a shared pin gets a coordinate-based label
		// rather than a resolved city name. Documented in the README.
		lat, lon = msg.Location.Latitude, msg.Location.Longitude
		name = fmt.Sprintf("Lokasi (%.4f, %.4f)", lat, lon)

	case strings.TrimSpace(msg.Text) != "":
		place, err := b.geocoder.Search(ctx, strings.TrimSpace(msg.Text))
		if err != nil {
			if errors.Is(err, openmeteo.ErrNoMatch) {
				b.reply(ctx, chatID, "Lokasi tidak ditemukan. Coba ketik nama kota lain, atau kirim lokasimu langsung.", locationRequestKeyboard())
				return
			}
			b.log.Error("geocode failed", "chat_id", chatID, "query", msg.Text, "error", err)
			b.reply(ctx, chatID, "Maaf, terjadi kesalahan saat mencari lokasi. Coba lagi sebentar lagi.", nil)
			return
		}
		name, lat, lon = place.DisplayName, place.Lat, place.Lon

	default:
		b.reply(ctx, chatID, "Ketik nama kota, atau kirim lokasimu langsung.", locationRequestKeyboard())
		return
	}

	locationID, err := b.db.UpsertLocation(ctx, name, lat, lon)
	if err != nil {
		b.log.Error("upsert location failed", "chat_id", chatID, "error", err)
		b.reply(ctx, chatID, "Maaf, terjadi kesalahan. Coba lagi sebentar lagi.", nil)
		return
	}

	userID, err := b.db.UserIDByChatID(ctx, chatID)
	if err != nil {
		b.replyNeedStart(ctx, chatID)
		return
	}

	if sess.LocationOnly {
		if err := b.db.ReplaceSubscription(ctx, userID, locationID); err != nil {
			b.log.Error("replace subscription failed", "chat_id", chatID, "user_id", userID, "error", err)
			b.reply(ctx, chatID, "Maaf, terjadi kesalahan. Coba lagi sebentar lagi.", nil)
			return
		}
		_ = b.sessions.Clear(ctx, chatID)
		b.log.Info("location changed", "chat_id", chatID, "user_id", userID, "location_id", locationID)
		b.reply(ctx, chatID, fmt.Sprintf("✅ Lokasi pantauan diganti ke %s.", name), telegram.ReplyKeyboardRemove{RemoveKeyboard: true})
		return
	}

	sess.LocationID = locationID
	sess.Step = redisstate.StepAwaitingCondition
	if err := b.sessions.Save(ctx, chatID, sess); err != nil {
		b.log.Error("save session failed", "chat_id", chatID, "error", err)
	}

	b.reply(ctx, chatID, fmt.Sprintf("📍 Lokasi terdaftar: %s", name), telegram.ReplyKeyboardRemove{RemoveKeyboard: true})
	b.reply(ctx, chatID, "Apakah kamu punya kondisi kesehatan khusus?", conditionKeyboard())
}

// handleCallback routes an inline-keyboard button press to the condition or
// sensitivity step, ignoring anything that doesn't match the chat's current
// step (e.g. a stale button from an abandoned flow).
func (b *Bot) handleCallback(ctx context.Context, cq *telegram.CallbackQuery) {
	if cq.Message == nil {
		return
	}
	chatID := cq.Message.Chat.ID

	defer func() {
		if err := b.tg.AnswerCallbackQuery(ctx, cq.ID, ""); err != nil {
			b.log.Error("answer callback query failed", "chat_id", chatID, "error", err)
		}
	}()

	sess, err := b.sessions.Get(ctx, chatID)
	if err != nil {
		b.log.Error("get session failed", "chat_id", chatID, "error", err)
		return
	}

	switch {
	case strings.HasPrefix(cq.Data, "cond:") && sess.Step == redisstate.StepAwaitingCondition:
		b.handleConditionCallback(ctx, chatID, sess, strings.TrimPrefix(cq.Data, "cond:"))
	case strings.HasPrefix(cq.Data, "sens:") && sess.Step == redisstate.StepAwaitingSensitivity:
		b.handleSensitivityCallback(ctx, chatID, sess, strings.TrimPrefix(cq.Data, "sens:"))
	default:
		b.log.Debug("ignoring stale/unexpected callback", "chat_id", chatID, "data", cq.Data, "step", sess.Step)
	}
}

func (b *Bot) handleConditionCallback(ctx context.Context, chatID int64, sess redisstate.Session, condition string) {
	sess.ConditionType = condition
	sess.Step = redisstate.StepAwaitingSensitivity
	if err := b.sessions.Save(ctx, chatID, sess); err != nil {
		b.log.Error("save session failed", "chat_id", chatID, "error", err)
	}

	b.reply(ctx, chatID, "Seberapa sensitif kamu terhadap polusi udara?", sensitivityKeyboard())
}

// handleSensitivityCallback finishes whichever flow started it: /ubahprofil
// (ProfileOnly) just records the new profile, while a fresh /start also
// activates the subscription for the location picked earlier in the flow.
func (b *Bot) handleSensitivityCallback(ctx context.Context, chatID int64, sess redisstate.Session, levelStr string) {
	level, err := strconv.Atoi(levelStr)
	if err != nil || level < 1 || level > 5 {
		b.log.Error("invalid sensitivity callback data", "chat_id", chatID, "data", levelStr)
		return
	}

	userID, err := b.db.UserIDByChatID(ctx, chatID)
	if err != nil {
		b.replyNeedStart(ctx, chatID)
		return
	}

	if err := b.db.InsertSensitivityProfile(ctx, userID, sess.ConditionType, int16(level)); err != nil {
		b.log.Error("insert sensitivity profile failed", "chat_id", chatID, "user_id", userID, "error", err)
		b.reply(ctx, chatID, "Maaf, terjadi kesalahan. Coba lagi sebentar lagi.", nil)
		return
	}

	if sess.ProfileOnly {
		_ = b.sessions.Clear(ctx, chatID)
		b.log.Info("profile updated", "chat_id", chatID, "user_id", userID, "condition", sess.ConditionType, "level", level)
		b.reply(ctx, chatID, "✅ Profil sensitivitas kamu sudah diperbarui.", nil)
		return
	}

	if err := b.db.ReplaceSubscription(ctx, userID, sess.LocationID); err != nil {
		b.log.Error("replace subscription failed", "chat_id", chatID, "user_id", userID, "error", err)
		b.reply(ctx, chatID, "Maaf, terjadi kesalahan saat menyimpan lokasi. Coba /ubahlokasi lagi.", nil)
		return
	}
	_ = b.sessions.Clear(ctx, chatID)

	locName, err := b.db.LocationName(ctx, sess.LocationID)
	if err != nil {
		locName = "lokasimu"
	}

	b.log.Info("onboarding complete", "chat_id", chatID, "user_id", userID, "location_id", sess.LocationID, "condition", sess.ConditionType, "level", level)
	b.reply(ctx, chatID, fmt.Sprintf(
		"🎉 Kamu terdaftar!\n\nLokasi: %s\nJagaNapas akan mengirim notifikasi otomatis saat kualitas udara berisiko untukmu.\n\nPerintah lain:\n/status — cek kondisi terkini\n/riwayat — 7 hari terakhir\n/ubahlokasi — ganti lokasi\n/ubahprofil — ubah sensitivitas\n/berhenti — berhenti berlangganan",
		locName,
	), nil)
}
