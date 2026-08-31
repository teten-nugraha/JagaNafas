# Diagrams — JagaNapas

Kumpulan diagram arsitektur & alur, diturunkan dari [PRD.md](../PRD.md).

| File | Isi |
|---|---|
| [01-high-level-diagram.md](01-high-level-diagram.md) | Gambaran arsitektur tingkat tinggi — sumber data, platform, user |
| [02-component-diagram.md](02-component-diagram.md) | Rincian tiap service/komponen dan koneksinya ke Redis Streams/Redis/Postgres |
| [03-data-flow-diagram.md](03-data-flow-diagram.md) | Perjalanan satu data point dari Open-Meteo sampai jadi alert |
| [04-application-flow-diagram.md](04-application-flow-diagram.md) | Sequence diagram interaksi user ↔ bot Telegram (onboarding, command, alert otomatis) |
| [05-diagram-non-it.md](05-diagram-non-it.md) | Versi sederhana tanpa istilah teknis, untuk audiens non-IT |

Semua diagram ditulis dalam format [Mermaid](https://mermaid.js.org/) — otomatis ter-render di GitHub, GitLab, dan sebagian besar editor markdown (termasuk VS Code dengan ekstensi Mermaid preview).

> **Catatan:** diagram di sini mengikuti versi PRD saat ini — **Redis Streams** sebagai message broker (menggantikan Apache Kafka pada draft awal), dan **Golang** sebagai bahasa implementasi (menggantikan Kotlin/Spring Boot). Lihat [PRD.md bagian 7](../PRD.md#7-komponen--tech-stack-mapping) untuk detail & alasan perubahan.

## Gambar (JPG)

Versi visual dari kelima diagram di atas, di-render sebagai gambar di folder [picts/](../picts/) — untuk dilihat cepat tanpa perlu renderer Mermaid, atau ditempel ke dokumen/slide:

| File | Diagram |
|---|---|
| [00-overview.jpg](../picts/00-overview.jpg) | Seluruh halaman (semua diagram sekaligus) |
| [01-high-level-diagram.jpg](../picts/01-high-level-diagram.jpg) | High-Level Diagram |
| [02-component-diagram.jpg](../picts/02-component-diagram.jpg) | Component Diagram |
| [03-data-flow-diagram.jpg](../picts/03-data-flow-diagram.jpg) | Data Flow Diagram |
| [04-application-flow-diagram.jpg](../picts/04-application-flow-diagram.jpg) | Application Flow Diagram |
| [05-diagram-non-it.jpg](../picts/05-diagram-non-it.jpg) | Diagram untuk Non-IT |

## Implementasi

Komponen yang digambarkan di sini sudah punya kode berjalan di [services/](../services/):

| Diagram menyebut | Kode di |
|---|---|
| Scheduler | [services/scheduler-service](../services/scheduler-service) |
| Risk Engine Consumer Group | [services/risk-engine-service](../services/risk-engine-service) |
| Telegram Notifier Service | [services/telegram-notifier-service](../services/telegram-notifier-service) |
| Telegram Bot Service | [services/bot-service](../services/bot-service) |

Semua dijalankan lewat [compose/docker-compose.yml](../compose/docker-compose.yml).
