// Package bot implements the JagaNapas Telegram Bot Service (PRD section 5
// & 6): /start onboarding (location + health profile), /status, /riwayat,
// /ubahlokasi, /ubahprofil, /berhenti — via long polling, so no public
// HTTPS endpoint is needed for local/compose deployment.
package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/jaganapas/bot-service/internal/config"
	"github.com/jaganapas/bot-service/internal/db"
	"github.com/jaganapas/bot-service/internal/openmeteo"
	"github.com/jaganapas/bot-service/internal/redisstate"
	"github.com/jaganapas/bot-service/internal/telegram"
)

type Bot struct {
	tg       *telegram.Client
	db       *db.DB
	sessions *redisstate.SessionStore
	scores   *redisstate.ScoreCacheReader
	geocoder *openmeteo.GeocodeClient
	cfg      *config.Config
	log      *slog.Logger
}

func New(cfg *config.Config, tg *telegram.Client, database *db.DB, sessions *redisstate.SessionStore, scores *redisstate.ScoreCacheReader, geocoder *openmeteo.GeocodeClient, log *slog.Logger) *Bot {
	return &Bot{
		tg: tg, db: database, sessions: sessions, scores: scores, geocoder: geocoder, cfg: cfg, log: log,
	}
}

// Run polls for updates until ctx is cancelled. Each getUpdates call blocks
// server-side for up to cfg.PollTimeoutSec, so this is a tight loop that's
// still gentle on the Telegram API and shuts down promptly on cancellation.
func (b *Bot) Run(ctx context.Context) {
	b.log.Info("bot polling starting", "poll_timeout_sec", b.cfg.PollTimeoutSec)

	var offset int64
	for {
		select {
		case <-ctx.Done():
			b.log.Info("bot polling stopping", "reason", ctx.Err())
			return
		default:
		}

		updates, err := b.tg.GetUpdates(ctx, offset, b.cfg.PollTimeoutSec)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			b.log.Error("getUpdates failed, backing off", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, u := range updates {
			offset = u.UpdateID + 1
			b.handleUpdate(ctx, u)
		}
	}
}

func (b *Bot) handleUpdate(ctx context.Context, u telegram.Update) {
	switch {
	case u.Message != nil:
		b.handleMessage(ctx, u.Message)
	case u.CallbackQuery != nil:
		b.handleCallback(ctx, u.CallbackQuery)
	}
}

// reply is a small convenience so handlers don't repeat error logging for
// every outbound send — a failed reply is logged but never fatal to the
// update loop.
func (b *Bot) reply(ctx context.Context, chatID int64, text string, keyboard any) {
	if err := b.tg.SendMessage(ctx, chatID, text, keyboard); err != nil {
		b.log.Error("send message failed", "chat_id", chatID, "error", err)
	}
}
