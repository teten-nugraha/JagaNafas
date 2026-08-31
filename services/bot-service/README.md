# bot-service

Telegram Bot Service dari PRD bagian 5, 6 & 7.1: `/start` onboarding
(lokasi + profil kesehatan), `/status`, `/riwayat`, `/ubahlokasi`,
`/ubahprofil`, `/berhenti`. Stack sama dengan `scheduler-service`,
`risk-engine-service`, dan `telegram-notifier-service`.

## Library yang dipakai

Sama seperti tiga service lain, seminimal mungkin:

| Library | Alasan |
|---|---|
| [`redis/go-redis/v9`](https://github.com/redis/go-redis) | State percakapan (FSM) per chat, dan baca cache skor yang ditulis `risk-engine-service` untuk `/status`. |
| [`gofiber/fiber/v2`](https://github.com/gofiber/fiber) | HTTP server untuk `/healthz` — sama seperti service lain. |
| [`jackc/pgx/v5`](https://github.com/jackc/pgx) | Client PostgreSQL — users, sensitivity_profiles, locations, user_subscriptions, riwayat. |
| [`caarlos0/env/v11`](https://github.com/caarlos0/env) | Parse env var → struct `Config`. |
| [`joho/godotenv`](https://github.com/joho/godotenv) | Load `.env` untuk dev lokal. |

**Sengaja tidak menambah library bot framework** (`go-telegram-bot-api`
dkk.): hanya 3 endpoint Telegram yang dipakai (`getUpdates`, `sendMessage`,
`answerCallbackQuery`), semuanya panggilan JSON POST/GET sederhana — lihat
`internal/telegram/client.go`. Memakai **long polling**, bukan webhook,
supaya tidak perlu endpoint HTTPS publik untuk jalan di lokal/compose (PRD
bagian 7 membolehkan keduanya: "via `go-telegram-bot-api` atau webhook
manual").

## Struktur

```
cmd/bot/main.go              - wiring, migrasi schema, graceful shutdown, -healthcheck
internal/config/             - load .env + env var → struct
internal/logging/            - slog JSON logger → stdout + logs/app.log
internal/db/                 - pgxpool, schema.sql (embed), semua query user/lokasi/profil/subscription/riwayat
internal/openmeteo/          - forward geocoding (nama kota → koordinat + nama tampilan)
internal/redisstate/         - session FSM per chat (Hash+TTL), baca cache:score:user:{id}
internal/telegram/           - client minimal: getUpdates, sendMessage, answerCallbackQuery
internal/bot/                - loop polling, dispatcher command/callback, alur onboarding, format pesan
internal/httpserver/         - Fiber app: /healthz (cek Redis & Postgres) + request logging
```

## Alur (PRD bagian 5)

State percakapan per chat disimpan di Redis (`bot:session:{chatId}`, TTL
`SESSION_TTL`, default 15 menit — flow yang ditinggalkan otomatis
kadaluarsa, tidak perlu proses cleanup terpisah):

1. `/start` → minta lokasi (ketik nama kota, atau tombol "Kirim Lokasi Saya")
2. Lokasi di-geocode via Open-Meteo → dikonfirmasi → tanya kondisi kesehatan (inline keyboard)
3. Tanya sensitivitas 1–5 (inline keyboard)
4. Simpan profil + aktifkan subscription → user mulai menerima notifikasi
5. `/ubahlokasi` menjalankan ulang langkah lokasi saja (mengganti subscription aktif — v1 satu lokasi per user, sesuai open question PRD bagian 16 yang belum diputuskan mendukung multi-lokasi)
6. `/ubahprofil` menjalankan ulang langkah kondisi+sensitivitas saja
7. `/status` baca `cache:score:user:{userId}` di Redis (ditulis `risk-engine-service`)
8. `/riwayat` query `risk_score_history` `HISTORY_DAYS` hari terakhir (default 7)
9. `/berhenti` menonaktifkan subscription aktif

## Keterbatasan yang diketahui (v1)

- **Berbagi lokasi via pin Telegram tidak di-reverse-geocode** — Open-Meteo
  hanya punya forward geocoding (nama → koordinat), jadi lokasi yang dikirim
  via tombol "Kirim Lokasi" diberi label `Lokasi (lat, lon)`, bukan nama
  kota. Mengetik nama kota tetap memberi nama tampilan lengkap (mis.
  "Bandung, Jawa Barat").
- **Satu lokasi aktif per user** — `/ubahlokasi` mengganti (bukan menambah)
  lokasi pantauan; multi-lokasi adalah open question di PRD bagian 16.
- **Offset polling tidak persisten** — disimpan di memori proses; restart
  service bisa memproses ulang beberapa update terakhir dari Telegram
  (idempotency risiko rendah untuk command bot, ditoleransi untuk v1).

## Menjalankan lokal

```bash
cp .env.example .env
# isi TELEGRAM_BOT_TOKEN dari @BotFather
go run ./cmd/bot
```

Butuh Redis & Postgres jalan (lihat `compose/docker-compose.yml` di root
repo). Tanpa `TELEGRAM_BOT_TOKEN`, service tetap start (dengan warning di
log) tapi `getUpdates` akan terus gagal 404 sampai token diisi.

## Build

```bash
go build -o bot ./cmd/bot
# atau
docker build -t jaganapas/bot-service .
```
