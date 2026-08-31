# Component Diagram — JagaNapas

Rincian komponen/service, teknologi yang dipakai, dan bagaimana mereka saling terhubung — mengikuti arsitektur di PRD bagian 6 & 7.

```mermaid
flowchart TB
    subgraph SchedulerSvc["Scheduler Service (Spring Boot)"]
        S1["Polling job tiap 15 menit"]
        S2["Open-Meteo API client"]
    end

    subgraph BotSvc["Telegram Bot Service (Spring Boot)"]
        B1["Command handler<br/>/start /status /ubahlokasi dst"]
        B2["Subscription manager"]
        B3["Geocoding lookup"]
    end

    subgraph ConsumerSvc["Kafka Consumer Service — Risk Engine (Spring Kafka)"]
        C1["Enrichment"]
        C2["Rolling trend calculator"]
        C3["Join subscription (Postgres)"]
        C4["Risk score calculator"]
        C5["Debounce checker"]
    end

    subgraph NotifierSvc["Telegram Notifier Service (Spring Kafka Consumer)"]
        N1["Consume risk-alerts"]
        N2["Format pesan"]
        N3["Telegram Bot API sendMessage"]
    end

    subgraph Infra["Infrastruktur"]
        KFK[("Kafka<br/>raw-environment-data<br/>risk-scores<br/>risk-alerts")]
        RDS[("Redis<br/>rolling window, debounce,<br/>cache skor, rate limit")]
        PGS[("PostgreSQL<br/>users, sensitivity_profiles,<br/>locations, user_subscriptions,<br/>risk_score_history, alert_history")]
    end

    S2 --> S1 --> KFK
    KFK --> C1 --> C2 --> C3 --> C4 --> C5 --> KFK
    C2 <--> RDS
    C5 <--> RDS
    C3 <--> PGS
    C4 -->|risk_score_history| PGS

    KFK --> N1 --> N2 --> N3
    N3 -->|alert_history| PGS

    B1 --> B2 --> PGS
    B3 --> PGS
    B1 -->|/status baca cache| RDS

    classDef svc fill:#e8f0fe,stroke:#4285f4;
    classDef infra fill:#fef7e0,stroke:#f9ab00;
    class SchedulerSvc,BotSvc,ConsumerSvc,NotifierSvc svc;
    class Infra infra;
```

**Catatan komponen:**
| Service | Tanggung jawab utama | Terhubung ke |
|---|---|---|
| Scheduler Service | Polling Open-Meteo, publish data mentah | Kafka |
| Telegram Bot Service | Interaksi user, kelola subscription/profil | Postgres, Redis, Kafka (implisit via Bot API) |
| Kafka Consumer (Risk Engine) | Enrichment, trend, scoring, debounce | Kafka, Redis, Postgres |
| Telegram Notifier Service | Kirim alert ke user | Kafka, Telegram Bot API, Postgres |
