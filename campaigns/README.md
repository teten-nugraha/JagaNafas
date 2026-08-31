# Campaigns — JagaNapas

Materi promosi untuk Instagram. Set pertama: carousel 7 slide memperkenalkan `@jaga_nafas_bot` — cara kerja, cara pakai, kota yang dicover, dan manfaatnya.

## Carousel: Kenalan dengan JagaNapas

Format 1080×1350 (4:5, portrait feed), urutkan sesuai nomor file saat upload sebagai carousel:

| File | Isi |
|---|---|
| [01-cover.png](01-cover.png) | Cover — nama bot, tagline, handle |
| [02-masalah.png](02-masalah.png) | Masalah yang diselesaikan — AQI mentah vs skor personal JagaNapas |
| [03-cara-kerja.png](03-cara-kerja.png) | Cara kerja — 4 langkah (daftar → dipantau → dihitung → notifikasi) |
| [04-cara-pakai.png](04-cara-pakai.png) | Cara pakai — alur onboarding + daftar command |
| [05-kota-yang-dicover.png](05-kota-yang-dicover.png) | 10 kota yang dicover (sesuai `scheduler-service`) |
| [06-manfaat.png](06-manfaat.png) | Manfaat — untuk siapa saja JagaNapas berguna |
| [07-cta.png](07-cta.png) | Call-to-action penutup |

## Caption saran

```
Udara di kotamu aman hari ini? 🌫️

JagaNapas bot Telegram yang mantau kualitas udara di kotamu 24 jam — otomatis,
dan personal sesuai kondisi kesehatanmu. Bukan angka AQI yang bikin bingung,
tapi skor risiko yang jelas + saran konkret.

✅ Gratis
✅ Notifikasi cuma pas penting, bukan spam
✅ Sudah cover 10 kota besar Indonesia

Mulai sekarang → @jaga_nafas_bot, ketik /start

#kualitasudara #ISPA #asma #polusiudara #telegrambot #kesehatan #indonesia
```

## Sumber data

- Daftar kota: [services/scheduler-service/internal/config/locations.json](../services/scheduler-service/internal/config/locations.json)
- Alur & command: [PRD.md bagian 5](../PRD.md#5-user-flow-telegram-bot)
- Palet & tipografi konsisten dengan [diagrams/](../diagrams/) (Fraunces + Manrope + JetBrains Mono, aksen teal "napas").
