# Diagram untuk Non-IT — Bagaimana JagaNapas Bekerja

Versi sederhana, tanpa istilah teknis — untuk dijelaskan ke stakeholder, keluarga, atau siapa pun yang tidak paham arsitektur software.

```mermaid
flowchart LR
    A(["🌫️ Udara di kotamu<br/>dipantau terus-menerus"]) --> B(["🧠 JagaNapas menghitung<br/>seberapa berbahaya udaranya<br/>untuk KAMU secara khusus"])
    B --> C{"Apakah berbahaya<br/>untuk kondisimu?"}
    C -->|Tidak| D(["😌 Tidak ada notifikasi<br/>Kamu tidak diganggu"])
    C -->|Ya| E(["📱 Kamu dapat pesan<br/>di Telegram"])
    E --> F(["✅ Kamu tahu harus apa:<br/>pakai masker, siap obat,<br/>atau tetap di rumah"])
```

## Cerita singkatnya

1. **Kamu daftar sekali** — kasih tahu bot di mana kamu tinggal, dan apakah kamu punya riwayat asma/gangguan napas.
2. **JagaNapas terus memantau** kualitas udara di lokasimu, sepanjang hari, tanpa kamu perlu buka aplikasi apapun.
3. **Kamu baru dihubungi kalau memang perlu** — bukan tiap jam, bukan spam. Hanya saat udara di sekitarmu benar-benar mulai berbahaya buat kondisi kesehatanmu.
4. **Pesannya jelas dan actionable** — bukan cuma angka teknis, tapi saran konkret: pakai masker, hindari keluar rumah, atau siapkan obat.

```mermaid
flowchart TD
    S1(["1. Daftar di Telegram<br/>kasih lokasi & kondisi kesehatan"]) --> S2(["2. JagaNapas jaga kamu<br/>24 jam, otomatis"])
    S2 --> S3(["3. Dapat peringatan<br/>hanya saat perlu"])
    S3 --> S4(["4. Ambil tindakan<br/>sesuai saran"])
    S4 --> S2
```

> Analogi: JagaNapas seperti punya teman yang selalu cek cuaca dan kualitas udara untukmu, lalu hanya menegur kamu saat benar-benar penting — dia tahu riwayat kesehatanmu jadi sarannya juga personal, bukan generik.
