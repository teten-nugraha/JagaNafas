# JagaNapas

**Sistem Peringatan Dini Risiko ISPA Berbasis Kualitas Udara & Cuaca via Bot Telegram**

> *"Jaga napasmu, sebelum udara mengejutkanmu."*

JagaNapas mengubah data AQI + cuaca mentah dari Open-Meteo menjadi **skor risiko personal** per user (bukan angka AQI generik), lalu mengirim notifikasi proaktif ke Telegram **hanya saat benar-benar dibutuhkan** — bukan spam tiap jam. Dibangun sebagai portofolio untuk menunjukkan sistem **event-driven, stateful, dan personalized** menggunakan Redis Streams dan PostgreSQL secara terintegrasi.

Detail lengkap kebutuhan produk ada di [PRD.md](PRD.md).

---

## Arsitektur

```
Open-Meteo API ──▶ Scheduler ──▶ Redis Stream: raw-environment-data
                                            │
                                            ▼
                                     Risk Engine ──▶ Redis Stream: risk-scores
                                     (skor personal        (histori skor)
                                      per subscriber)
                                            │
                                            ▼
                              Redis Stream: risk-alerts
                                            │
                                            ▼
                              Telegram Notifier ──▶ Telegram Bot API
                                            │
                                            ▼
                                    Postgres: alert_history

                    Telegram Bot Service ◀──▶ User (/start, /status, /riwayat, ...)
```

Diagram visual lengkap (Mermaid + JPG) ada di [diagrams/](diagrams/) dan [picts/](picts/).

## Tech Stack

| Komponen | Teknologi |
|---|---|
| Bahasa | **Golang** (tanpa framework HTTP berat — `net/http`/Fiber seperlunya) |
| Message broker & stream processing | **Redis Streams** (`XADD`/`XREADGROUP`, consumer group per service) |
| State & cache | **Redis** (rolling window trend, debounce alert, cache skor) |
| Database | **PostgreSQL** (via `pgx`) |
| Bot | **Telegram Bot API** (long polling, client minimal tanpa library framework) |
| Sumber data | **Open-Meteo API** (AQI + cuaca + geocoding, gratis tanpa API key) |
| Deployment | Docker Compose (lokal) |

Alasan tiap pilihan (termasuk kenapa Redis Streams menggantikan Kafka, dan kenapa Golang menggantikan Kotlin/Spring Boot) dijelaskan di [PRD.md bagian 7](PRD.md#7-komponen--tech-stack-mapping).

## Services

Empat service Go independen di [services/](services/), masing-masing dengan `README.md` sendiri yang menjelaskan detail implementasi & library yang dipakai:

| Service | Peran | Port (compose) |
|---|---|---|
| [scheduler-service](services/scheduler-service) | Polling Open-Meteo tiap 15 menit, publish ke `stream:raw-environment-data` | 8082 |
| [risk-engine-service](services/risk-engine-service) | Consumer group: enrichment, trend, personalized risk scoring, debounce, publish `risk-scores`/`risk-alerts` | 8083 |
| [telegram-notifier-service](services/telegram-notifier-service) | Consumer group: kirim alert via Telegram Bot API, catat `alert_history` | 8084 |
| [bot-service](services/bot-service) | `/start` onboarding, `/status`, `/riwayat`, `/ubahlokasi`, `/ubahprofil`, `/berhenti` | 8085 |

Semua service mengekspos `/healthz` dan menulis log JSON terstruktur ke stdout + `logs/app.log`.

## Menjalankan Lokal

```bash
# 1. Siapkan environment
cp compose/.env.example compose/.env
# isi TELEGRAM_BOT_TOKEN di compose/.env (dari @BotFather)

# 2. Jalankan seluruh stack
cd compose
docker compose up -d --build

# 3. Cek status
docker compose ps
curl http://localhost:8082/healthz  # scheduler-service
curl http://localhost:8083/healthz  # risk-engine-service
curl http://localhost:8084/healthz  # telegram-notifier-service
curl http://localhost:8085/healthz  # bot-service
```

Lalu chat ke bot Telegram kamu dan kirim `/start`.

**Infrastruktur yang ikut terangkat:** PostgreSQL (`5432`), Redis (`6379`), RedisInsight UI (`8081`) — lihat [compose/docker-compose.yml](compose/docker-compose.yml). Konfigurasi tiap service ada di [compose/env/](compose/env/) (nilai statis) digabung dengan `compose/.env` (kredensial & token).

Untuk menjalankan satu service saja di luar Docker (mis. saat development), lihat bagian "Menjalankan lokal" di README masing-masing service — semua butuh Redis & Postgres yang sama (bisa tetap pakai yang dari `docker compose up`).

## Struktur Proyek

```
PRD.md                  - Product Requirements Document (sumber kebenaran arsitektur & keputusan desain)
diagrams/                - Diagram arsitektur & alur (Mermaid)
picts/                   - Render JPG dari diagram di atas
compose/
  docker-compose.yml      - Orkestrasi seluruh stack
  env/                     - Konfigurasi statis per service
  .env.example             - Template kredensial (Postgres, Telegram bot token)
services/
  scheduler-service/
  risk-engine-service/
  telegram-notifier-service/
  bot-service/
```

## Status

Draft aktif — lihat [PRD.md bagian 14 (Roadmap)](PRD.md#14-roadmap--milestone-pengerjaan) untuk progres milestone, dan [PRD.md bagian 16 (Open Questions)](PRD.md#16-open-questions) untuk keputusan desain yang masih terbuka (mis. dukungan multi-lokasi per user).
