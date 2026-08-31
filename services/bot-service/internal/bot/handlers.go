package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jaganapas/bot-service/internal/redisstate"
	"github.com/jaganapas/bot-service/internal/telegram"
)

// handleMessage routes a plain (non-callback) update: a command if the
// text starts with "/", otherwise input for whatever flow step the chat is
// currently in (PRD section 5).
func (b *Bot) handleMessage(ctx context.Context, msg *telegram.Message) {
	chatID := msg.Chat.ID

	if strings.HasPrefix(msg.Text, "/") {
		username := ""
		if msg.From != nil {
			username = msg.From.Username
		}
		b.handleCommand(ctx, chatID, username, msg.Text)
		return
	}

	sess, err := b.sessions.Get(ctx, chatID)
	if err != nil {
		b.log.Error("get session failed", "chat_id", chatID, "error", err)
		return
	}

	if sess.Step == redisstate.StepAwaitingLocation {
		b.handleLocationInput(ctx, chatID, msg, sess)
		return
	}

	b.reply(ctx, chatID, "Ketik /start untuk mulai, atau /status untuk cek kondisi udara terkini.", nil)
}

func (b *Bot) handleCommand(ctx context.Context, chatID int64, username, text string) {
	cmd := strings.ToLower(strings.Fields(text)[0])
	cmd = strings.SplitN(cmd, "@", 2)[0] // strip "@BotName" suffix in group chats

	switch cmd {
	case "/start":
		b.cmdStart(ctx, chatID, username)
	case "/status":
		b.cmdStatus(ctx, chatID)
	case "/riwayat":
		b.cmdRiwayat(ctx, chatID)
	case "/ubahlokasi":
		b.cmdUbahLokasi(ctx, chatID)
	case "/ubahprofil":
		b.cmdUbahProfil(ctx, chatID)
	case "/berhenti":
		b.cmdBerhenti(ctx, chatID)
	default:
		b.reply(ctx, chatID, "Perintah tidak dikenali.\n\nPerintah yang tersedia:\n/status — cek kondisi terkini\n/riwayat — 7 hari terakhir\n/ubahlokasi — ganti lokasi\n/ubahprofil — ubah sensitivitas\n/berhenti — berhenti berlangganan", nil)
	}
}

func (b *Bot) cmdStart(ctx context.Context, chatID int64, username string) {
	userID, err := b.db.UpsertUser(ctx, chatID, username)
	if err != nil {
		b.log.Error("upsert user failed", "chat_id", chatID, "error", err)
		b.reply(ctx, chatID, "Maaf, terjadi kesalahan. Coba lagi sebentar lagi.", nil)
		return
	}
	b.log.Info("user started", "chat_id", chatID, "user_id", userID)

	sess := redisstate.Session{Step: redisstate.StepAwaitingLocation}
	if err := b.sessions.Save(ctx, chatID, sess); err != nil {
		b.log.Error("save session failed", "chat_id", chatID, "error", err)
	}

	b.reply(ctx, chatID,
		"👋 Selamat datang di JagaNapas!\n\n\"Jaga napasmu, sebelum udara mengejutkanmu.\"\n\nDi mana lokasi yang ingin kamu pantau? Ketik nama kota, atau kirim lokasimu langsung.",
		locationRequestKeyboard(),
	)
}

func (b *Bot) cmdUbahLokasi(ctx context.Context, chatID int64) {
	if _, err := b.db.UserIDByChatID(ctx, chatID); err != nil {
		b.replyNeedStart(ctx, chatID)
		return
	}

	sess := redisstate.Session{Step: redisstate.StepAwaitingLocation, LocationOnly: true}
	if err := b.sessions.Save(ctx, chatID, sess); err != nil {
		b.log.Error("save session failed", "chat_id", chatID, "error", err)
	}

	b.reply(ctx, chatID, "Mau pantau lokasi mana? Ketik nama kota, atau kirim lokasimu langsung.", locationRequestKeyboard())
}

func (b *Bot) cmdUbahProfil(ctx context.Context, chatID int64) {
	if _, err := b.db.UserIDByChatID(ctx, chatID); err != nil {
		b.replyNeedStart(ctx, chatID)
		return
	}

	sess := redisstate.Session{Step: redisstate.StepAwaitingCondition, ProfileOnly: true}
	if err := b.sessions.Save(ctx, chatID, sess); err != nil {
		b.log.Error("save session failed", "chat_id", chatID, "error", err)
	}

	b.reply(ctx, chatID, "Apakah kamu punya kondisi kesehatan khusus?", conditionKeyboard())
}

func (b *Bot) cmdBerhenti(ctx context.Context, chatID int64) {
	userID, err := b.db.UserIDByChatID(ctx, chatID)
	if err != nil {
		b.replyNeedStart(ctx, chatID)
		return
	}

	if err := b.db.Unsubscribe(ctx, userID); err != nil {
		b.log.Error("unsubscribe failed", "chat_id", chatID, "user_id", userID, "error", err)
		b.reply(ctx, chatID, "Maaf, terjadi kesalahan. Coba lagi sebentar lagi.", nil)
		return
	}
	_ = b.sessions.Clear(ctx, chatID)

	b.log.Info("user unsubscribed", "chat_id", chatID, "user_id", userID)
	b.reply(ctx, chatID, "Kamu sudah berhenti berlangganan notifikasi JagaNapas. Ketik /start kapan saja untuk mulai lagi.", telegram.ReplyKeyboardRemove{RemoveKeyboard: true})
}

func (b *Bot) cmdStatus(ctx context.Context, chatID int64) {
	userID, err := b.db.UserIDByChatID(ctx, chatID)
	if err != nil {
		b.replyNeedStart(ctx, chatID)
		return
	}

	cached, err := b.scores.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, redisstate.ErrNoCachedScore) {
			b.reply(ctx, chatID, "Belum ada data skor risiko untuk lokasimu. Data biasanya muncul dalam 15 menit setelah kamu subscribe.", nil)
			return
		}
		b.log.Error("get cached score failed", "chat_id", chatID, "user_id", userID, "error", err)
		b.reply(ctx, chatID, "Maaf, terjadi kesalahan. Coba lagi sebentar lagi.", nil)
		return
	}

	locName, err := b.db.LocationName(ctx, cached.LocationID)
	if err != nil {
		locName = fmt.Sprintf("Lokasi #%d", cached.LocationID)
	}

	b.reply(ctx, chatID, formatStatus(locName, cached), nil)
}

func (b *Bot) cmdRiwayat(ctx context.Context, chatID int64) {
	userID, err := b.db.UserIDByChatID(ctx, chatID)
	if err != nil {
		b.replyNeedStart(ctx, chatID)
		return
	}

	since := time.Now().AddDate(0, 0, -b.cfg.HistoryDays)
	rows, err := b.db.RecentHistory(ctx, userID, since, b.cfg.HistoryLimit)
	if err != nil {
		b.log.Error("recent history failed", "chat_id", chatID, "user_id", userID, "error", err)
		b.reply(ctx, chatID, "Maaf, terjadi kesalahan. Coba lagi sebentar lagi.", nil)
		return
	}
	if len(rows) == 0 {
		b.reply(ctx, chatID, fmt.Sprintf("Belum ada riwayat skor risiko dalam %d hari terakhir.", b.cfg.HistoryDays), nil)
		return
	}

	b.reply(ctx, chatID, formatHistory(rows, b.cfg.HistoryDays), nil)
}

func (b *Bot) replyNeedStart(ctx context.Context, chatID int64) {
	b.reply(ctx, chatID, "Kamu belum terdaftar. Ketik /start untuk mulai.", nil)
}
