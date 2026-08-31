# scheduler-service

Polling service dari PRD bagian 6 & 7.1: tiap `POLL_INTERVAL` (default 15 menit),
menarik AQI + cuaca dari Open-Meteo untuk semua lokasi terdaftar, lalu publish
tiap pembacaan ke Redis Stream `stream:raw-environment-data` untuk dikonsumsi
Risk Engine.

## Library yang dipakai

Dipilih seminimal mungkin — hanya yang benar-benar mempercepat/menyederhanakan development:

| Library | Alasan |
|---|---|
| [`redis/go-redis/v9`](https://github.com/redis/go-redis) | Client Redis Streams (`XADD`) — wajib sesuai PRD bagian 7 & 9. |
| [`gofiber/fiber/v2`](https://github.com/gofiber/fiber) | HTTP server untuk `/healthz` — middleware `recover` + logging request siap pakai, routing lebih ringkas dibanding menulis mux manual. |
| [`caarlos0/env/v11`](https://github.com/caarlos0/env) | Parse env var langsung ke struct `Config` (dengan default value & tipe `time.Duration`), menghindari boilerplate `os.Getenv` + parsing manual. |
| [`joho/godotenv`](https://github.com/joho/godotenv) | Load file `.env` untuk dev lokal, konsisten dengan pola `.env` yang sudah dipakai di `compose/`. |

Sisanya (JSON, concurrency, scheduling interval, structured logging) pakai
standard library Go saja (`encoding/json`, `log/slog`, `time.Ticker`, `context`)
— tidak perlu cron library tambahan untuk interval polling sesederhana ini.

## Struktur

```
cmd/scheduler/main.go       - wiring, graceful shutdown, -healthcheck flag
internal/config/            - load .env + env var → struct, daftar lokasi default (embed)
internal/logging/           - slog JSON logger → stdout + logs/app.log
internal/openmeteo/         - client AQI + cuaca Open-Meteo (fetch paralel, logged)
internal/scheduler/         - ticker loop, poll semua lokasi secara concurrent
internal/stream/            - publisher Redis Stream (XADD)
internal/httpserver/        - Fiber app: /healthz + request logging + recover
```

## Logging

Log terstruktur JSON (`log/slog`) ditulis ke **stdout** (`docker logs` tetap
jalan) sekaligus ke file di `LOG_FILE` (default `logs/app.log`), supaya bisa
diperiksa langsung tanpa container harus hidup. Level diatur via `LOG_LEVEL`
(`debug`/`info`/`warn`/`error`).

Yang dicatat untuk troubleshooting:
- Startup: config yang ter-load, status koneksi Redis.
- Tiap request HTTP (`/healthz`): method, path, status, latency, IP.
- Tiap panggilan Open-Meteo (level `debug`): host, path, status, durasi; body
  response disertakan kalau status bukan 200.
- Tiap poll lokasi: durasi fetch & publish, `stream_id` hasil `XADD` (untuk
  korelasi ke consumer di sisi Risk Engine), atau error lengkap kalau gagal.
- Tiap poll cycle: jumlah lokasi sukses/gagal, total durasi.
- Shutdown: sinyal yang diterima, hasil graceful shutdown HTTP server.

## Menjalankan lokal

```bash
cp .env.example .env
go run ./cmd/scheduler
```

Butuh Redis jalan (lihat `compose/docker-compose.yml` di root repo). Log akan
muncul di terminal dan di `logs/app.log`.

## Konfigurasi lokasi

Default 10 kota padat penduduk Indonesia ada di `internal/config/locations.json`
(di-embed ke binary). Untuk override tanpa rebuild, set `LOCATIONS_FILE` ke path
JSON dengan struktur yang sama:

```json
[{ "id": 1, "name": "Bandung", "lat": -6.9175, "lon": 107.6191 }]
```

## Build

```bash
go build -o scheduler ./cmd/scheduler
# atau
docker build -t jaganapas/scheduler-service .
```
