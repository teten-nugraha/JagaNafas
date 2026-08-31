# High-Level Diagram — JagaNapas

Gambaran arsitektur tingkat tinggi: dari sumber data cuaca/AQI, diproses lewat pipeline event-driven, sampai notifikasi diterima user di Telegram.

```mermaid
flowchart LR
    subgraph External["Sumber Eksternal"]
        OM["Open-Meteo API<br/>(AQI + Cuaca)"]
        TG["Telegram"]
    end

    subgraph Core["JagaNapas Platform"]
        direction TB
        SCH["Scheduler<br/>(poll tiap 15 menit)"]
        KAFKA["Apache Kafka<br/>(message broker)"]
        RISK["Risk Engine<br/>(Kafka Consumer)"]
        NOTIF["Telegram Notifier"]
        BOTSVC["Telegram Bot Service"]
        REDIS[("Redis<br/>cache & rolling window")]
        PG[("PostgreSQL<br/>user, profil, histori")]
    end

    OM -->|data mentah| SCH
    SCH -->|publish| KAFKA
    KAFKA -->|consume| RISK
    RISK <--> REDIS
    RISK <--> PG
    RISK -->|publish alert| KAFKA
    KAFKA -->|consume alert| NOTIF
    NOTIF -->|sendMessage| TG
    TG <-->|user interaction| BOTSVC
    BOTSVC <--> PG
    BOTSVC <--> REDIS
    TG -->|notifikasi risiko udara| User(("User"))
    User -->|/start, lokasi, profil| TG
```

**Ringkasan alur:**
1. Scheduler menarik data AQI & cuaca dari Open-Meteo tiap 15 menit.
2. Data mentah dipublish ke Kafka, lalu diproses Risk Engine (hitung tren via Redis, cocokkan dengan subscription di Postgres).
3. Skor risiko yang lolos threshold & debounce dikirim sebagai alert ke Telegram.
4. User berinteraksi dua arah dengan bot (daftar lokasi, atur profil sensitivitas, cek status) via Telegram Bot Service.
