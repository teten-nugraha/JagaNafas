# Diagrams — JagaNapas

Kumpulan diagram arsitektur & alur, diturunkan dari [PRD.md](../PRD.md).

| File | Isi |
|---|---|
| [01-high-level-diagram.md](01-high-level-diagram.md) | Gambaran arsitektur tingkat tinggi — sumber data, platform, user |
| [02-component-diagram.md](02-component-diagram.md) | Rincian tiap service/komponen dan koneksinya ke Kafka/Redis/Postgres |
| [03-data-flow-diagram.md](03-data-flow-diagram.md) | Perjalanan satu data point dari Open-Meteo sampai jadi alert |
| [04-application-flow-diagram.md](04-application-flow-diagram.md) | Sequence diagram interaksi user ↔ bot Telegram (onboarding, command, alert otomatis) |
| [05-diagram-non-it.md](05-diagram-non-it.md) | Versi sederhana tanpa istilah teknis, untuk audiens non-IT |

Semua diagram ditulis dalam format [Mermaid](https://mermaid.js.org/) — otomatis ter-render di GitHub, GitLab, dan sebagian besar editor markdown (termasuk VS Code dengan ekstensi Mermaid preview).
