# Application Flow Diagram — JagaNapas

Sequence diagram interaksi user dengan bot Telegram, dari onboarding sampai menerima notifikasi (PRD bagian 5 & 11).

```mermaid
sequenceDiagram
    actor U as User
    participant TG as Telegram
    participant Bot as Bot Service
    participant PG as PostgreSQL
    participant Sched as Scheduler
    participant Risk as Risk Engine
    participant Notif as Notifier Service

    U->>TG: /start
    TG->>Bot: update /start
    Bot->>TG: minta lokasi (teks / share Location)
    U->>TG: kirim lokasi
    TG->>Bot: lokasi diterima
    Bot->>Bot: geocoding (Open-Meteo)
    Bot->>PG: simpan locations
    Bot->>TG: konfirmasi lokasi terdaftar

    Bot->>TG: tanya kondisi kesehatan (inline keyboard)
    U->>TG: pilih kondisi (mis. asma sedang)
    Bot->>TG: tanya level sensitivitas (1-5)
    U->>TG: pilih level
    Bot->>PG: simpan users + sensitivity_profiles + user_subscriptions
    Bot->>TG: "Kamu terdaftar, notifikasi aktif"

    Note over Sched,Notif: --- Proses berjalan otomatis di background ---
    loop tiap 15 menit
        Sched->>Sched: fetch AQI + cuaca (Open-Meteo)
        Sched->>Risk: publish raw-environment-data
        Risk->>Risk: hitung trend, risk score per subscriber
        alt skor Berisiko/Bahaya & lolos debounce
            Risk->>Notif: publish risk-alerts
            Notif->>TG: sendMessage (format PRD #11)
            TG->>U: ⚠️ Notifikasi kualitas udara
        end
    end

    U->>TG: /status
    TG->>Bot: request status
    Bot->>Bot: baca cache skor (Redis)
    Bot->>TG: kirim skor risiko terkini
    TG->>U: tampilkan status

    U->>TG: /riwayat
    Bot->>PG: query risk_score_history (7 hari)
    Bot->>TG: kirim ringkasan histori
    TG->>U: tampilkan riwayat

    U->>TG: /berhenti
    Bot->>PG: set is_active=false pada subscription
    Bot->>TG: konfirmasi berhenti
    TG->>U: "Kamu sudah berhenti berlangganan"
```

**Command yang tersedia setelah onboarding:**
`/status`, `/ubahlokasi`, `/ubahprofil`, `/berhenti`, `/riwayat` — semua dijelaskan di PRD bagian 5.
