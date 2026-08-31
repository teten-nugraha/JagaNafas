# risk-engine-service

Consumer group dari PRD bagian 6, 9 & 10: membaca `stream:raw-environment-data`,
menghitung risk score personal per subscriber, lalu publish ke
`stream:risk-scores` (selalu) dan `stream:risk-alerts` (kalau lolos threshold +
debounce). Stack sama dengan `scheduler-service`, ditambah Postgres.

## Library yang dipakai

Sama seperti `scheduler-service`, ditambah client Postgres — tetap seminimal
mungkin, hanya yang benar-benar mempercepat/menyederhanakan development:

| Library | Alasan |
|---|---|
| [`redis/go-redis/v9`](https://github.com/redis/go-redis) | Client Redis Streams (`XREADGROUP`/`XACK`/`XAUTOCLAIM`) & state (Sorted Set, TTL, Hash) — wajib sesuai PRD bagian 7 & 9. |
| [`gofiber/fiber/v2`](https://github.com/gofiber/fiber) | HTTP server untuk `/healthz` — sama seperti scheduler-service. |
| [`jackc/pgx/v5`](https://github.com/jackc/pgx) | Client PostgreSQL (`pgxpool`) — dipilih sesuai PRD bagian 7 (`pgx`/`sqlc`); `sqlc` sendiri tidak dipakai karena butuh tool codegen terpisah yang tidak sepadan untuk jumlah query sekecil ini — SQL ditulis langsung. |
| [`caarlos0/env/v11`](https://github.com/caarlos0/env) | Parse env var → struct `Config`. |
| [`joho/godotenv`](https://github.com/joho/godotenv) | Load `.env` untuk dev lokal. |

Tidak ada library migrasi terpisah (mis. `golang-migrate`) — skema
(`internal/db/schema.sql`, disalin persis dari PRD bagian 8) dijalankan lewat
`CREATE TABLE/INDEX IF NOT EXISTS` setiap startup lewat `pgx`, jadi idempotent
tanpa tool tambahan.

## Struktur

```
cmd/riskengine/main.go      - wiring, migrasi schema, graceful shutdown, -healthcheck
internal/config/            - load .env + env var → struct
internal/logging/           - slog JSON logger → stdout + logs/app.log
internal/db/                - pgxpool, schema.sql (embed), query subscriber & histori
internal/redisstate/        - rolling trend (Sorted Set), debounce (TTL), cache skor (Hash)
internal/riskscore/         - normalisasi, discomfort index, trend weight, kategori, formatter pesan
internal/stream/            - parse raw-environment-data, publisher risk-scores/risk-alerts
internal/consumer/          - XREADGROUP loop + XAUTOCLAIM sweep, orkestrasi pipeline PRD bagian 10
internal/httpserver/        - Fiber app: /healthz (cek Redis & Postgres) + request logging
```

## Logika Risk Score (PRD bagian 10)

PRD menetapkan pipeline (normalisasi → discomfort index → trend weight →
multiplier sensitivitas → kategori 0-100) tapi tidak formula/breakpoint
eksak. Pilihan konkret yang dipakai (didokumentasikan di
`internal/riskscore/score.go`):

- **Normalisasi PM2.5/PM10** → breakpoint linear-piecewise berbasis pedoman
  WHO 24 jam & ambang ISPU "Tidak Sehat"/"Berbahaya", diambil nilai terburuk
  dari kedua polutan (konvensi AQI umum).
- **Discomfort index** → *Thom's discomfort index* sederhana dari suhu +
  kelembapan, jadi penambah/pengurang kecil (maks ±10 poin) — kualitas udara
  tetap sinyal utama.
- **Trend** → PM2.5 sekarang vs rata-rata rolling 3 jam (Redis Sorted Set,
  di-prune via `ZREMRANGEBYSCORE`); naik ≥20% → bobot naik (maks 1.2x), turun
  ≥20% → bobot turun (min 0.9x).
- **Multiplier sensitivitas** → linear `sensitivity_level` 1–5 ke 1.0–1.8,
  persis seperti disebut PRD ("1.0 untuk umum, sampai 1.8 untuk asma berat").

## Catatan penyelarasan `locationId`

`raw-environment-data.locationId` diasumsikan sama dengan `locations.id` di
Postgres (sesuai penamaan field di PRD bagian 9). Selama belum ada
`bot-service` yang mendaftarkan lokasi (mengisi tabel `locations` &
`user_subscriptions`), consumer akan tetap jalan normal tapi tidak menemukan
subscriber — pesan tetap diproses & di-ACK, log `"no active subscribers for
location"` di level debug.

## Idempotency & reliability (PRD bagian 13)

- Consumer group `cg-risk-engine` (nama dapat diatur via `CONSUMER_GROUP`).
- Pesan hanya di-`XACK` setelah pipeline selesai; error infra (parse gagal,
  Redis/Postgres unreachable) membiarkan pesan tetap pending untuk direplay.
- Sweep `XAUTOCLAIM` berkala (`CLAIM_INTERVAL`, default 30s) mengklaim ulang
  pesan pending yang idle lebih dari `CLAIM_MIN_IDLE` (default 1 menit) —
  menangani kasus consumer crash sebelum sempat ACK.
- Debounce alert dicek lewat Redis key `alert:lastsent:{userId}` sebelum
  publish ke `risk-alerts`, sesuai PRD.

## Menjalankan lokal

```bash
cp .env.example .env
go run ./cmd/riskengine
```

Butuh Redis & Postgres jalan (lihat `compose/docker-compose.yml` di root
repo) — schema akan otomatis dibuat saat startup. Log muncul di terminal dan
`logs/app.log`.

## Build

```bash
go build -o riskengine ./cmd/riskengine
# atau
docker build -t jaganapas/risk-engine-service .
```
