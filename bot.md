# Panduan Bot Telegram — JagaNapas

Dokumen ini menjelaskan dua hal:

1. **Cara membuat & mengonfigurasi bot Telegram** (`@JagaNapasBot`) dari nol.
2. **Alur subscription user berdasarkan lokasi** — bagaimana bot mengumpulkan lokasi user, menyimpannya, dan menggunakannya untuk mengirim notifikasi risiko udara yang dipersonalisasi.

Referensi arsitektur lengkap ada di [PRD.md](PRD.md) bagian 5 (User Flow) dan bagian 6 (Arsitektur Sistem).

---

## 1. Membuat Bot via BotFather

1. Buka Telegram, cari **@BotFather**.
2. Kirim `/newbot`.
3. Ikuti instruksi:
   - **Nama tampilan** (boleh spasi & emoji): `JagaNapas`
   - **Username** (harus unik, berakhiran `bot`): `JagaNapasBot` atau `jaga_napas_bot` jika sudah dipakai.
4. BotFather akan membalas dengan **bot token**, formatnya:
   ```
   123456789:AAH_contoh_token_jangan_dipakai_asli
   ```
   Simpan token ini sebagai secret (env var `TELEGRAM_BOT_TOKEN`) — **jangan pernah di-commit ke git**.

### 1.1 Konfigurasi tambahan di BotFather

| Command BotFather | Kegunaan |
|---|---|
| `/setdescription` | Deskripsi bot yang muncul sebelum user chat pertama kali. |
| `/setabouttext` | Teks singkat di halaman profil bot. |
| `/setuserpic` | Upload logo/icon bot. |
| `/setcommands` | Daftarkan command supaya muncul di menu "/" Telegram. |
| `/setprivacy` | Pastikan **Disable** jika bot perlu baca semua pesan di grup (untuk v1, bot dipakai di private chat saja, jadi boleh default **Enable**). |
| `/setjoingroups` | **Disable**, karena JagaNapas didesain untuk private chat per user, bukan grup. |

Isi untuk `/setcommands` (kirim ke BotFather, pilih bot-nya, lalu paste blok ini):

```
start - Mulai & daftar lokasi pantauan
status - Cek risk score lokasi saat ini
ubahlokasi - Ganti atau tambah lokasi pantauan
ubahprofil - Update tingkat sensitivitas kesehatan
riwayat - Lihat 7 hari terakhir risk score
berhenti - Berhenti menerima notifikasi
```

---

## 2. Mode Integrasi: Webhook vs Long Polling

| Mode | Kapan dipakai | Catatan |
|---|---|---|
| **Long polling** (`getUpdates`) | Development lokal | Tidak butuh domain publik/HTTPS. Cocok untuk `docker-compose` lokal. |
| **Webhook** (`setWebhook`) | Production (Railway/Fly.io) | Wajib HTTPS. Telegram push update ke endpoint kita — lebih hemat resource daripada polling terus-menerus. |

### 2.1 Setup Webhook (production)

```bash
curl -F "url=https://jaganapas.up.railway.app/telegram/webhook" \
     -F "secret_token=${TELEGRAM_WEBHOOK_SECRET}" \
     "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/setWebhook"
```

- `secret_token` dicek di header `X-Telegram-Bot-Api-Secret-Token` pada setiap request masuk, untuk memastikan request benar-benar dari Telegram.
- Endpoint Spring Boot yang menerima webhook: `POST /telegram/webhook`, menerima payload `Update` dari Telegram Bot API dan meneruskannya ke handler command.
- Cek status webhook: `GET https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getWebhookInfo`.
- Hapus webhook (kembali ke polling saat development): `GET .../deleteWebhook`.

### 2.2 Long Polling (lokal)

Gunakan library `org.telegram:telegrambots-longpolling` (atau `telegrambots-spring-boot-starter`) yang menjalankan `getUpdates` di background thread — tidak perlu domain publik, cukup `TELEGRAM_BOT_TOKEN` di `.env` lokal.

---

## 3. Alur Subscription Berdasarkan Lokasi

Ini inti dari fitur personalisasi JagaNapas: user harus mendaftarkan **lokasi** dan **profil sensitivitas** sebelum bisa menerima notifikasi. Alur di bawah adalah detail teknis dari [PRD.md bagian 5](PRD.md#5-user-flow-telegram-bot).

### 3.1 Diagram Alur

```
User                     Telegram Bot Service              Open-Meteo              Postgres
 |                              |                               |                      |
 |--/start--------------------->|                               |                      |
 |                              |--upsert users(telegram_chat_id)---------------------->|
 |<--"Kirim lokasi kamu"--------|                               |                      |
 |   (ReplyKeyboard: tombol     |                               |                      |
 |    "Bagikan Lokasi" +        |                               |                      |
 |    input teks nama kota)     |                               |                      |
 |                              |                               |                      |
 |--kirim Location (lat/lon)--->|                               |                      |
 |   ATAU teks "Bandung"        |--geocoding search------------>|                      |
 |                              |<--lat, lon, nama resmi---------|                      |
 |                              |--upsert locations(lat, lon)-------------------------->|
 |<--"Lokasi: Bandung, Jabar.   |                               |                      |
 |    Benar?" (inline Ya/Ubah)--|                               |                      |
 |--tekan "Ya"----------------->|                               |                      |
 |                              |                               |                      |
 |<--tanya kondisi kesehatan----|                               |                      |
 |   (inline keyboard)          |                               |                      |
 |--pilih "Asma sedang"-------->|--upsert sensitivity_profiles-------------------------->|
 |                              |                               |                      |
 |<--tanya level sensitivitas---|                               |                      |
 |--pilih "Tinggi (4/5)"------->|--update sensitivity_profiles-------------------------->|
 |                              |--insert user_subscriptions(user_id, location_id,       |
 |                              |    is_active=true)-------------------------------------->|
 |<--"Subscription aktif! 🎉"---|                               |                      |
```

Setelah `user_subscriptions` aktif, lokasi tersebut otomatis ikut dipoll oleh **Scheduler Service** (lihat [services/scheduler-service](services/scheduler-service)) — tapi hanya lokasi yang **benar-benar disubscribe minimal satu user** yang perlu dipoll, supaya tidak boros API call ke Open-Meteo untuk kota yang tidak ada penggunanya.

> **Catatan implementasi:** Scheduler saat ini polling dari daftar statis di `application.yml` (`jaganapas.scheduler.locations`). Untuk v1 penuh, daftar ini sebaiknya diganti jadi query dinamis: `SELECT DISTINCT l.* FROM locations l JOIN user_subscriptions us ON us.location_id = l.id WHERE us.is_active = true`, di-cache di memori/Redis dan di-refresh tiap kali ada subscription baru atau tiap siklus polling.

### 3.2 Langkah Detail

1. **`/start`**
   - Bot upsert baris di tabel `users` berdasarkan `telegram_chat_id` (didapat dari `update.message.chat.id`).
   - Bot balas dengan `ReplyKeyboardMarkup` berisi satu tombol khusus:
     ```json
     {
       "keyboard": [[{ "text": "📍 Bagikan Lokasi", "request_location": true }]],
       "resize_keyboard": true,
       "one_time_keyboard": true
     }
     ```
   - User juga boleh mengetik nama kota manual sebagai alternatif (bot mendeteksi apakah update berisi `message.location` atau `message.text`).

2. **Menentukan koordinat**
   - **Kalau user share Location**: Telegram mengirim `message.location.latitude` & `message.location.longitude` langsung — tidak perlu geocoding.
   - **Kalau user ketik nama kota**: bot memanggil **Open-Meteo Geocoding API** (gratis, tanpa API key):
     ```
     GET https://geocoding-api.open-meteo.com/v1/search?name=Bandung&count=1&language=id
     ```
     Ambil `results[0].latitude`, `results[0].longitude`, `results[0].name`, `results[0].admin1` (provinsi) untuk konfirmasi ke user.

3. **Konfirmasi lokasi**
   - Bot kirim inline keyboard `[Ya, benar] [Ganti lokasi]` supaya user bisa koreksi kalau geocoding salah kota (misal ada beberapa kota dengan nama sama).

4. **Upsert ke `locations`**
   - Cek `UNIQUE(lat, lon)` — kalau lokasi sudah ada (dipakai user lain), pakai `location_id` yang sama, jangan duplikat baris.

5. **Profil sensitivitas**
   - Inline keyboard kondisi kesehatan → simpan ke `sensitivity_profiles.condition_type`.
   - Inline keyboard skala 1–5 atau Rendah/Sedang/Tinggi → simpan ke `sensitivity_profiles.sensitivity_level`.

6. **Aktivasi subscription**
   - Insert/update `user_subscriptions(user_id, location_id, is_active=true)`.
   - Bot kirim konfirmasi akhir + ringkasan (lokasi, kondisi, sensitivitas) supaya user bisa langsung sadar apa yang terdaftar.

### 3.3 Command Lanjutan yang Bergantung pada Subscription

| Command | Query yang dijalankan |
|---|---|
| `/status` | Baca cache skor terkini dari Redis (`skor:{locationId}`) untuk lokasi aktif user. |
| `/ubahlokasi` | Ulangi langkah 2–4 di atas, lalu update `user_subscriptions.location_id` (atau insert baris baru jika mendukung multi-lokasi di v2). |
| `/ubahprofil` | Ulangi langkah 5, update baris `sensitivity_profiles` yang sudah ada. |
| `/riwayat` | `SELECT * FROM risk_score_history WHERE user_id = ? ORDER BY computed_at DESC LIMIT ... (7 hari)`. |
| `/berhenti` | `UPDATE user_subscriptions SET is_active = false WHERE user_id = ?` — data historis tetap disimpan, tidak dihapus (soft unsubscribe). |

---

## 4. Environment Variables yang Dibutuhkan

```env
TELEGRAM_BOT_TOKEN=123456789:AA...
TELEGRAM_WEBHOOK_SECRET=random-string-panjang   # hanya untuk mode webhook
TELEGRAM_WEBHOOK_URL=https://jaganapas.up.railway.app/telegram/webhook
```

Simpan di `.env` (sudah masuk `.gitignore`), jangan pernah hardcode token di kode atau `application.yml`.

---

## 5. Testing Lokal

1. Jalankan `docker-compose up` untuk Redis + Postgres (lihat [compose/docker-compose.yml](compose/docker-compose.yml)).
2. Set `TELEGRAM_BOT_TOKEN` di `.env` lokal, jalankan Telegram Bot Service dalam mode **long polling** (tidak perlu ngrok/webhook untuk dev).
3. Chat langsung ke bot dari akun Telegram pribadi, jalankan `/start`, coba share lokasi & ketik nama kota, pastikan baris tersimpan di Postgres (`users`, `locations`, `sensitivity_profiles`, `user_subscriptions`).
4. Untuk uji webhook sebelum deploy, bisa pakai `ngrok http 8080` lalu `setWebhook` ke URL ngrok sementara.
