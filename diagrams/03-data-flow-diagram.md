# Data Flow Diagram — JagaNapas

Menelusuri perjalanan satu data point AQI/cuaca dari sumber sampai jadi notifikasi, termasuk baca/tulis ke Redis & Postgres di tiap tahap (PRD bagian 6, 9, 10).

```mermaid
flowchart TD
    A["Open-Meteo API<br/>PM2.5, PM10, suhu, kelembapan"] --> B["Scheduler: fetch tiap 15 menit"]
    B --> C["Topic: raw-environment-data<br/>key=locationId"]

    C --> D["Consumer: Enrichment"]
    D --> E["Ambil rolling window 3 jam<br/>dari Redis (per lokasi)"]
    E --> F["Hitung trend<br/>(naik/turun/stabil)"]
    F --> G["Update rolling window di Redis"]

    F --> H["Query subscription aktif<br/>di lokasi ini (Postgres)"]
    H --> I["Untuk tiap user subscriber:"]

    I --> J["Ambil sensitivity_profile user"]
    J --> K["Normalisasi PM2.5/PM10 → 0-100"]
    K --> L["Hitung discomfort index<br/>(suhu + kelembapan)"]
    L --> M["Kalikan multiplier sensitivitas<br/>(1.0 - 1.8)"]
    M --> N["Risk Score final (0-100)<br/>+ kategori"]

    N --> O["Simpan ke risk_score_history<br/>(Postgres)"]
    N --> P["Cache skor terkini per lokasi<br/>(Redis)"]
    N --> Q["Topic: risk-scores<br/>key=userId"]

    N --> R{"Skor >= Berisiko<br/>(61-100)?"}
    R -->|Tidak| STOP1(["Selesai, tidak ada alert"])
    R -->|Ya| S["Cek Redis: alert:lastsent:{userId}"]

    S --> T{"Sudah <3 jam sejak<br/>alert terakhir?"}
    T -->|Ya, dan kenaikan <2 kategori| STOP2(["Debounce: alert ditahan"])
    T -->|Tidak, atau naik >=2 kategori| U["Topic: risk-alerts<br/>key=userId"]

    U --> V["Notifier Service consume"]
    V --> W["Format pesan personalisasi"]
    W --> X["Telegram sendMessage"]
    X --> Y["Simpan ke alert_history (Postgres)"]
    X --> Z["Update Redis: alert:lastsent:{userId}"]
    X --> END(["User menerima notifikasi"])
```

**Poin penting alur data:**
- Setiap event mentah bisa menghasilkan banyak `risk-scores` (satu per user subscriber di lokasi itu), tapi hanya sebagian lolos jadi `risk-alerts`.
- Redis berperan sebagai *state* jangka pendek (rolling window, debounce), Postgres sebagai *system of record* jangka panjang (histori, profil).
- Idempotency: sebelum publish ke `risk-alerts`, wajib cek debounce state di Redis agar tidak reprocessing menyebabkan pesan dobel (NFR di PRD bagian 13).
