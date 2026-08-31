# PRD — JagaNapas
### Sistem Peringatan Dini Risiko ISPA Berbasis Kualitas Udara & Cuaca via Bot Telegram

**Versi:** 1.0
**Status:** Draft
**Pemilik Produk:** (Nama kamu)
**Tanggal:** 31 Agustus 2026

---

## 1. Latar Belakang & Problem Statement

Indonesia punya masalah kualitas udara yang signifikan di banyak kota besar, terutama saat musim kemarau/kabut asap. Penderita ISPA (Infeksi Saluran Pernapasan Akut), asma, atau kondisi pernapasan sensitif lainnya sering tidak sadar bahwa kondisi udara di sekitar mereka sedang memburuk sampai gejala muncul.

Aplikasi cuaca/AQI yang ada sekarang umumnya:
- Menampilkan angka AQI mentah yang sulit diinterpretasi orang awam ("PM2.5: 87 µg/m³" — bahaya atau tidak?)
- Tidak mempertimbangkan kondisi kesehatan personal user
- Butuh user aktif membuka aplikasi, bukan proaktif memberi tahu

**JagaNapas** hadir sebagai bot Telegram yang mengubah data mentah AQI + cuaca menjadi **skor risiko personal** dan mengirim notifikasi proaktif hanya saat benar-benar dibutuhkan.

---

## 2. Tujuan Produk (Goals)

1. Memberikan **early warning** kondisi udara berisiko ke penderita ISPA/asma berdasarkan lokasi tempat tinggal mereka.
2. Personalisasi risk score berdasarkan tingkat sensitivitas masing-masing user (bukan AQI generik).
3. Menghindari notification fatigue — alert hanya dikirim saat relevan (bukan spam tiap jam).
4. Sebagai portofolio: menunjukkan kemampuan membangun sistem **event-driven, stateful, dan personalized** menggunakan Redis Streams dan Postgres secara terintegrasi dan bermakna (bukan sekadar CRUD).

### Non-Goals (Out of Scope untuk v1)
- Tidak menggantikan diagnosis medis — hanya alat bantu informasi.
- Tidak mendukung multi-platform notifikasi (WhatsApp, push app) di v1 — fokus Telegram saja.
- Tidak menyediakan peta interaktif / visualisasi kompleks di v1.

---

## 3. Target Pengguna

| Persona | Kebutuhan |
|---|---|
| Penderita asma/ISPA kronis | Tahu kapan harus pakai masker/hindari keluar rumah |
| Orang tua dengan anak balita/lansia di rumah | Dapat notifikasi untuk keputusan aktivitas keluarga |
| Masyarakat umum di kota rawan polusi/kabut asap | Info kualitas udara real-time tanpa perlu cek manual |

---

## 4. Nama Produk & Branding

**Nama:** JagaNapas
**Bot Telegram:** `@JagaNapasBot`
**Tagline:** *"Jaga napasmu, sebelum udara mengejutkanmu."*

Alasan pemilihan nama: singkat, dua suku kata familiar dalam Bahasa Indonesia (jaga = lindungi, napas = pernapasan), langsung menjelaskan fungsi produk, dan mudah diingat sebagai username bot maupun nama repository GitHub.

---

## 5. User Flow (Telegram Bot)

```
1. User cari @JagaNapasBot di Telegram → /start
2. Bot minta lokasi:
   - User kirim nama kota (teks), atau
   - User share Location lewat fitur Telegram
3. Bot konfirmasi lokasi terdaftar (geocoding via Open-Meteo)
4. Bot tanya kondisi kesehatan (inline keyboard):
   - Tidak ada kondisi khusus / Asma ringan / Asma sedang-berat / ISPA berulang / Lainnya
5. Bot tanya seberapa sensitif (skala 1-5 atau kategori Rendah/Sedang/Tinggi)
6. Subscription tersimpan → user mulai menerima notifikasi otomatis
7. User bisa command lain:
   /status  → cek risk score lokasi saat ini
   /ubahlokasi → ganti/tambah lokasi pantauan
   /ubahprofil → update sensitivitas
   /berhenti → unsubscribe
   /riwayat → lihat 7 hari terakhir risk score di lokasinya
```

---

## 6. Arsitektur Sistem

```
                    +---------------------------+
                    |  Scheduler (Spring)        |
                    |  poll tiap 15 menit         |
                    +-------------+---------------+
                                  |
                                  v
                    [Open-Meteo API: AQI + Cuaca]
                                  |
                                  v
              Redis Stream: raw-environment-data
                                  |
                                  v
                +-----------------------------------+
                |     Risk Engine Consumer Group      |
                |  (Spring Data Redis: StreamListener)|
                |  - enrich data                      |
                |  - hitung rolling trend (Redis      |
                |    Sorted Set/List sbg time window)  |
                |  - join dgn subscription (Postgres) |
                |  - hitung personalized risk score   |
                |  - debounce check (Redis key TTL)   |
                +-----------------+-------------------+
                                  |
              +-------------------+-------------------+
              v                                        v
  Redis Stream: risk-scores                Redis Stream: risk-alerts
              |                                        |
              v                                        v
   [Redis: cache skor per lokasi]          [Telegram Notifier Service]
   [Postgres: risk_score_history]          (Redis Stream Consumer Group →
                                             Telegram Bot API sendMessage)
                                                        |
                                                        v
                                             [Postgres: alert_history]

        +----------------------------------------+
        |         Telegram Bot Service             |
        |  (Spring Boot + Telegram Bot API/Webhook) |
        |  - handle /start, lokasi, profil          |
        |  - simpan ke Postgres (users, subs)       |
        |  - baca Redis untuk /status               |
        +--------------------------------------------+
```

---

## 7. Komponen & Tech Stack Mapping

| Komponen | Teknologi | Peran |
|---|---|---|
| Bahasa & Framework | **Kotlin + Spring Boot 4** | Semua service |
| Bot Interface | **Telegram Bot API** (via `telegrambots` lib atau webhook manual) | Interaksi user |
| Message Broker & Stream Processing | **Redis Streams** (`XADD`/`XREADGROUP`, consumer group per service) | Pipeline data environment → risk scoring → alert, sekaligus komputasi stateful |
| State & Cache | **Redis** (Sorted Set/List untuk rolling window, String dgn TTL untuk debounce, Hash untuk cache skor) | Rolling window trend, debounce alert, cache skor per lokasi, rate limit |
| Database | **PostgreSQL** | User, profil sensitivitas, subscription, histori skor, histori alert |
| Sumber Data | **Open-Meteo API** (AQI + Cuaca, gratis tanpa API key) | Data mentah |
| Deployment | Docker Compose (lokal), Railway/Fly.io (demo live) | Hosting |

> **Catatan desain:** v1 menggunakan **Redis Streams** sebagai message broker sekaligus tulang punggung stream processing, menggantikan Apache Kafka untuk menyederhanakan operasional (satu infrastruktur — Redis — untuk streaming, state, dan cache, tanpa perlu Zookeeper/KRaft & broker terpisah). Setiap tahap pipeline (`raw-environment-data` → `risk-scores` → `risk-alerts`) adalah Redis Stream dengan consumer group masing-masing (`XREADGROUP` + `XACK`) sehingga tetap punya jaminan at-least-once delivery dan kemampuan replay. Rolling window trend dan state lain dihitung manual memakai struktur data Redis (Sorted Set/List), bukan state store terkelola seperti Kafka Streams — trade-off ini dipilih demi kesederhanaan operasional, dengan konsekuensi state management (idempotency, TTL, cleanup) menjadi tanggung jawab kode aplikasi. Pendekatan ini tetap mempertahankan karakteristik event-driven & stateful processing sebagai nilai jual portofolio, dengan kompleksitas infrastruktur yang jauh lebih rendah dibanding Kafka.

### 7.1 Topologi Deployment

Diagram di bagian 6 menunjukkan **4 komponen logis**:

1. **Scheduler** — polling Open-Meteo, publish ke `stream:raw-environment-data`.
2. **Risk Engine** — consumer group yang menghitung trend & risk score.
3. **Telegram Notifier** — consumer group yang mengirim alert ke Telegram.
4. **Telegram Bot Service** — handle command user & webhook Telegram.

Untuk v1, keempatnya dijalankan sebagai **satu modular monolith** (1 Spring Boot app, 1 Docker image, 4 package/module terpisah: `scheduler`, `riskengine`, `notifier`, `bot`) — bukan 4 service terpisah. Alasan:
- Menyederhanakan deployment di free-tier hosting (1 container, bukan 4).
- Konsumen Redis Stream tetap berjalan sebagai background listener (`@Component` + `StreamMessageListenerContainer`) di dalam proses yang sama, sehingga karakteristik event-driven & stateful tetap terlihat di kode tanpa overhead operasional multi-service.
- Batas antar modul tetap dijaga rapi (package-per-modul, tidak saling import internal) sehingga bisa dipecah jadi service terpisah di v2 jika diperlukan (misal untuk showcase scaling independen).

---

## 8. Skema Data (PostgreSQL)

```sql
users (
  id BIGSERIAL PRIMARY KEY,
  telegram_chat_id BIGINT UNIQUE NOT NULL,
  username VARCHAR,
  created_at TIMESTAMP DEFAULT now()
)

sensitivity_profiles (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT REFERENCES users(id),
  condition_type VARCHAR, -- 'asma_ringan', 'asma_berat', 'ispa_berulang', 'umum'
  sensitivity_level SMALLINT, -- 1-5
  updated_at TIMESTAMP DEFAULT now()
)

locations (
  id BIGSERIAL PRIMARY KEY,
  city_name VARCHAR NOT NULL,
  lat DOUBLE PRECISION,
  lon DOUBLE PRECISION,
  UNIQUE(lat, lon)
)

user_subscriptions (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT REFERENCES users(id),
  location_id BIGINT REFERENCES locations(id),
  is_active BOOLEAN DEFAULT true,
  created_at TIMESTAMP DEFAULT now()
)

risk_score_history (
  id BIGSERIAL PRIMARY KEY,
  location_id BIGINT REFERENCES locations(id),
  user_id BIGINT REFERENCES users(id),
  pm25 DOUBLE PRECISION,
  pm10 DOUBLE PRECISION,
  temperature DOUBLE PRECISION,
  humidity DOUBLE PRECISION,
  risk_score DOUBLE PRECISION,
  trend VARCHAR, -- 'naik', 'turun', 'stabil'
  computed_at TIMESTAMP DEFAULT now()
)

alert_history (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT REFERENCES users(id),
  location_id BIGINT REFERENCES locations(id),
  risk_score DOUBLE PRECISION,
  message TEXT,
  sent_at TIMESTAMP DEFAULT now()
)
```

---

## 9. Redis Streams

| Stream Key | Consumer Group | Payload (fields) | Deskripsi |
|---|---|---|---|
| `stream:raw-environment-data` | `cg-risk-engine` | `locationId, pm25, pm10, temp, humidity, timestamp` | Data mentah hasil polling |
| `stream:risk-scores` | `cg-score-writer` | `userId, locationId, score, trend, timestamp` | Skor risiko per user-lokasi, semua nilai (untuk histori) |
| `stream:risk-alerts` | `cg-telegram-notifier` | `userId, locationId, score, message, timestamp` | Hanya yang lolos threshold & debounce |

> Setiap consumer membaca via `XREADGROUP` dan meng-`XACK` setelah pesan berhasil diproses; pesan yang gagal diproses (pending) dapat direplay dengan `XCLAIM`/`XAUTOCLAIM` untuk menjamin at-least-once delivery.

---

## 10. Logika Perhitungan Risk Score

**Input:** PM2.5, PM10 (AQI), suhu, kelembapan, trend 3 jam terakhir, profil sensitivitas user.

**Langkah:**
1. Normalisasi PM2.5/PM10 ke skala 0–100 berdasarkan standar WHO/BMKG.
2. Hitung *discomfort index* dari suhu + kelembapan (heat index sederhana).
3. Hitung **trend** dengan membandingkan nilai saat ini vs rata-rata 3 jam terakhir (disimpan sebagai Redis Sorted Set per `locationId`, di-score dengan timestamp, entry lama di luar window dibuang dgn `ZREMRANGEBYSCORE`) — kenaikan cepat diberi bobot lebih tinggi.
4. Kalikan dengan **multiplier sensitivitas** user (1.0 untuk umum, sampai 1.8 untuk asma berat).
5. Hasil akhir: skor 0–100, dikategorikan:
   - 0–30 → Aman
   - 31–60 → Waspada
   - 61–80 → Berisiko
   - 81–100 → Bahaya

**Alert Rule:**
- Kirim notifikasi jika skor masuk kategori "Berisiko" atau "Bahaya" **DAN**
- Belum ada alert terkirim ke user tersebut dalam 3 jam terakhir (debounce, dicek dari Redis key `alert:lastsent:{userId}`) **ATAU** skor naik ≥2 kategori dari alert terakhir (override debounce untuk kasus darurat).

---

## 11. Contoh Format Notifikasi Telegram

```
⚠️ JagaNapas — Peringatan Kualitas Udara

Lokasi: Bandung, Jawa Barat
Skor Risiko: 74/100 (Berisiko)
PM2.5: 92 µg/m³ (naik dari 65 µg/m³, 3 jam terakhir)
Suhu: 31°C, Kelembapan: 58%

Karena kamu tercatat memiliki riwayat asma sedang,
disarankan untuk:
• Gunakan masker N95 jika keluar rumah
• Hindari aktivitas fisik berat di luar ruangan
• Pastikan obat pereda asma tersedia

Ketik /status untuk cek kondisi terkini.
```

---

## 12. API Internal (opsional, untuk keperluan admin/monitoring)

| Endpoint | Method | Deskripsi |
|---|---|---|
| `/api/locations/{id}/current` | GET | Skor risiko terkini suatu lokasi (baca Redis) |
| `/api/users/{id}/history` | GET | Histori skor risiko user (baca Postgres) |
| `/api/admin/subscriptions` | GET | Daftar semua subscription aktif (untuk debugging) |
| `/actuator/health` | GET | Health check service |

---

## 13. Non-Functional Requirements

- **Latency:** dari polling data sampai notifikasi terkirim maksimal < 2 menit.
- **Reliability:** consumer group harus idempotent — reprocessing pesan Redis Stream yang sama (misal setelah crash sebelum `XACK`) tidak boleh mengirim alert dobel (cek debounce state di Redis sebelum publish ke stream `risk-alerts`); pending entries di-monitor & direplay via `XAUTOCLAIM`.
- **Observability:** logging terstruktur (JSON) untuk setiap tahap pipeline; expose metrics dasar via Spring Actuator, termasuk consumer lag (`XPENDING`/`XINFO GROUPS`) per stream.
- **Testability:** integration test untuk consumer/producer Redis Streams dan interaksi Postgres/Redis menggunakan **Testcontainers**.

---

## 14. Roadmap / Milestone Pengerjaan

| Fase | Deliverable |
|---|---|
| 1. Setup | Docker Compose (Redis, Postgres), skeleton Spring Boot project, skema DB |
| 2. Data Ingestion | Scheduler polling Open-Meteo → publish ke `stream:raw-environment-data` |
| 3. Bot Dasar | `/start`, input lokasi, input profil sensitivitas, simpan ke Postgres |
| 4. Risk Engine | Consumer group: enrichment, trend calculation (Redis), risk scoring, publish ke `stream:risk-scores` |
| 5. Alert Engine | Debounce logic, publish ke `stream:risk-alerts`, Telegram Notifier Service kirim pesan |
| 6. Command Tambahan | `/status`, `/riwayat`, `/ubahlokasi`, `/ubahprofil`, `/berhenti` |
| 7. Testing | Unit test + integration test (Testcontainers) |
| 8. Deployment | Deploy ke Railway/Fly.io, setup webhook Telegram production |
| 9. Dokumentasi | README dengan arsitektur, cara run lokal, demo bot link, penjelasan trade-off desain |

---

## 15. Success Metrics (untuk portofolio, bukan bisnis)

- Sistem berjalan stabil 24/7 di free-tier hosting selama minimal 2 minggu demo.
- Bisa didemokan end-to-end: dari perubahan data AQI (simulasi) sampai notifikasi Telegram diterima < 2 menit.
- README dan kode terdokumentasi cukup jelas untuk dijelaskan dalam sesi interview 15–20 menit.

---

## 16. Open Questions

- Apakah perlu dukungan multi-lokasi per user (misal pantau rumah + kantor) di v1, atau cukup 1 lokasi dulu?
- Apakah histori 7 hari cukup, atau perlu retensi lebih panjang untuk analisis tren musiman?
- Apakah perlu fallback sumber data AQI kedua (misal OpenWeatherMap) jika Open-Meteo down?
