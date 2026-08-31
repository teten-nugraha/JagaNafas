# telegram-notifier-service

Consumer group dari PRD bagian 6 & 9: membaca `stream:risk-alerts`, mengirim
pesan via Telegram Bot API `sendMessage`, lalu mencatat `alert_history` —
**hanya setelah pengiriman berhasil**. Stack sama dengan `scheduler-service`
dan `risk-engine-service`.

## Library yang dipakai

Sama seperti `scheduler-service`/`risk-engine-service`, seminimal mungkin:

| Library | Alasan |
|---|---|
| [`redis/go-redis/v9`](https://github.com/redis/go-redis) | Client Redis Streams (`XREADGROUP`/`XACK`/`XAUTOCLAIM`) — wajib sesuai PRD bagian 7 & 9. |
| [`gofiber/fiber/v2`](https://github.com/gofiber/fiber) | HTTP server untuk `/healthz` — sama seperti dua service lain. |
| [`jackc/pgx/v5`](https://github.com/jackc/pgx) | Client PostgreSQL — lookup `telegram_chat_id` & tulis `alert_history`. |
| [`caarlos0/env/v11`](https://github.com/caarlos0/env) | Parse env var → struct `Config`. |
| [`joho/godotenv`](https://github.com/joho/godotenv) | Load `.env` untuk dev lokal. |

**Sengaja tidak menambah library bot Telegram** (`go-telegram-bot-api` dkk.):
`sendMessage` cuma satu panggilan POST JSON sederhana, dan service ini hanya
*mengirim* (menerima update/command adalah tugas `bot-service`, belum
dibangun) — jadi `net/http` biasa (lihat `internal/telegram/client.go`) sudah
cukup tanpa menambah dependency yang sebagian besar fiturnya tidak dipakai.

## Struktur

```
cmd/notifier/main.go        - wiring, migrasi schema, graceful shutdown, -healthcheck
internal/config/            - load .env + env var → struct
internal/logging/           - slog JSON logger → stdout + logs/app.log
internal/db/                - pgxpool, schema.sql (embed), lookup chat id, insert alert_history
internal/telegram/          - client sendMessage minimal (net/http)
internal/stream/            - parse risk-alerts (userId, locationId, score, message, timestamp)
internal/consumer/          - XREADGROUP loop + XAUTOCLAIM sweep
internal/httpserver/        - Fiber app: /healthz (cek Redis & Postgres) + request logging
```

## Kenapa `alert_history` pindah dari risk-engine-service

Diagram arsitektur PRD bagian 6 menaruh `[Postgres: alert_history]` **setelah**
Telegram Notifier Service, bukan di titik publish `risk-alerts`. Sebagai
bagian dari membangun service ini, `risk-engine-service` diperbaiki (baris
`InsertAlertHistory` dihapus dari `internal/consumer/consumer.go` &
`internal/db/db.go`) supaya `alert_history` hanya dicatat sekali, di sini,
setelah pesan benar-benar terkirim — bukan dobel di kedua service.

## Idempotency & reliability (PRD bagian 13)

- Consumer group `cg-telegram-notifier` (nama dapat diatur via `CONSUMER_GROUP`).
- Pesan hanya di-`XACK` setelah selesai diproses. Dibedakan tiga kasus:
  - **Sukses kirim** → simpan `alert_history`, ACK.
  - **Gagal permanen** (bot diblokir user / chat tidak ditemukan — HTTP 403
    atau 400 dengan deskripsi terkait) → log warning, ACK tanpa retry (retry
    tidak akan pernah berhasil).
  - **Gagal transient** (Postgres/Telegram API sementara tidak terjangkau,
    rate limit, dll.) → tidak di-ACK, pesan tetap pending untuk direplay.
- Sweep `XAUTOCLAIM` berkala (`CLAIM_INTERVAL`, default 30s) mengklaim ulang
  pesan pending yang idle lebih dari `CLAIM_MIN_IDLE` (default 1 menit).

**Keterbatasan yang diketahui (v1):** rate limit Telegram (HTTP 429 dengan
`retry_after`) saat ini diperlakukan sebagai kegagalan transient biasa
(menunggu sweep `XAUTOCLAIM` berikutnya), bukan menghormati `retry_after`
secara eksplisit — cukup untuk skala portofolio, bisa ditingkatkan di v2.

## Menjalankan lokal

```bash
cp .env.example .env
# isi TELEGRAM_BOT_TOKEN dari @BotFather
go run ./cmd/notifier
```

Butuh Redis & Postgres jalan (lihat `compose/docker-compose.yml` di root
repo). Tanpa `TELEGRAM_BOT_TOKEN`, service tetap start (dengan warning di
log) tapi setiap `sendMessage` akan gagal.

## Build

```bash
go build -o notifier ./cmd/notifier
# atau
docker build -t jaganapas/telegram-notifier-service .
```
