# satpam-go

Versi backend Go untuk project Satpam API, dibuat terpisah dari `satpam-app` supaya migrasi bisa bertahap.

## Endpoint awal

- `GET /healthz`
- `POST /api/v1/auth/login`
- `GET /api/v1/auth/me`
- `GET /api/v1/users`
- `GET /api/v1/recent-activities`
- `GET /api/v1/patrol/progress?attendanceId=...`

## Menjalankan

1. Copy `.env.example` menjadi `.env.local` atau `.env`
2. Pastikan PostgreSQL aktif dan schema dari project lama sudah tersedia
3. Jalankan:

```bash
go mod tidy
go run ./cmd/api
```

Server default berjalan di `http://localhost:8080`.

Untuk hot reload saat development, jalankan:

```bash
air
```

Repo ini sudah memakai `.air.toml` yang membuild entrypoint `./cmd/api`.

## Upload foto lokal

- `POST /api/v1/uploads/attendance`
- `POST /api/v1/uploads/patrol`

### Langkah setup

1. Buka `.env.local`
2. Atur `STORAGE_ROOT`
3. Atur `UPLOAD_MAX_BYTES` jika limit upload ingin diubah
4. Jalankan `go run ./cmd/api`
5. Upload file ke endpoint upload
6. Simpan nilai `photoUrl` dari response ke endpoint attendance atau patrol yang sudah ada

Contoh `.env.local`:

```env
STORAGE_ROOT=D:\satpam-storage
UPLOAD_MAX_BYTES=3145728
```

Gunakan `multipart/form-data` dengan field:

- `file`
- `placeId`
- `userId`
- `date` format `YYYY-MM-DD`
- `name` opsional

File yang diterima:

- `image/jpeg`
- `image/webp`

Hasil file disimpan ke:

```text
{STORAGE_ROOT}/places/{placeId}/users/{userId}/{date}/{attendance|patrol}/...
```

Foto bisa diakses lewat:

```text
/uploads/places/{placeId}/users/{userId}/{date}/{attendance|patrol}/...
```

Contoh request PowerShell:

```powershell
$headers = @{ Authorization = "Bearer TOKEN_KAMU" }
Invoke-RestMethod `
  -Method Post `
  -Uri "http://localhost:8080/api/v1/uploads/attendance" `
  -Headers $headers `
  -Form @{
    file = Get-Item "C:\temp\foto.jpg"
    placeId = "11111111-1111-1111-1111-111111111111"
    userId = "22222222-2222-2222-2222-222222222222"
    date = "2026-03-13"
    name = "checkin"
  }
```

Contoh response:

```json
{
  "objectKey": "places/11111111-1111-1111-1111-111111111111/users/22222222-2222-2222-2222-222222222222/2026-03-13/attendance/checkin-a1b2c3d4e5f6.jpg",
  "photoUrl": "/uploads/places/11111111-1111-1111-1111-111111111111/users/22222222-2222-2222-2222-222222222222/2026-03-13/attendance/checkin-a1b2c3d4e5f6.jpg",
  "mimeType": "image/jpeg",
  "size": 83421
}
```

Retention attendance otomatis menyimpan bulan berjalan dan 1 bulan sebelumnya. Foto attendance yang lebih lama akan dihapus saat startup dan lewat cleanup harian.

## Auto Checkout Attendance

Backend bisa checkout attendance otomatis setelah lewat `end_time shift + grace period`.

Tambahkan env berikut:

```env
AUTO_CHECKOUT_ENABLED=true
AUTO_CHECKOUT_GRACE_MINUTES=5
AUTO_CHECKOUT_POLL_SECONDS=60
AUTO_CHECKOUT_SYSTEM_PHOTO_URL=/uploads/system/attendance/check-out-by-system.svg
AUTO_CHECKOUT_SYSTEM_NOTE=Check out by system
```

Perilaku:

- hanya attendance yang sudah `check_in_at` dan belum `check_out_at`
- hanya attendance yang punya `shift_id`
- waktu auto checkout dihitung dari `attendance_date + shift.end_time + grace`
- untuk shift lewat tengah malam, `end_time` dianggap hari berikutnya
- `check_out_photo_url` diisi default asset sistem jika masih kosong
- `note` diisi `Check out by system` jika masih kosong

Default asset sistem akan dibuat otomatis di:

```text
/uploads/system/attendance/check-out-by-system.svg
```

### File yang perlu diubah kalau behavior storage berubah

- `.env.local`
  Untuk ganti `STORAGE_ROOT` dan `UPLOAD_MAX_BYTES`
- `internal/config/config.go`
  Untuk menambah config storage baru
- `internal/media/service.go`
  Untuk ubah struktur folder, nama file, format file yang diterima, dan retention cleanup
- `internal/media/handler.go`
  Untuk ubah validasi upload request
- `internal/httpapi/router.go`
  Untuk ubah endpoint upload atau route file statis `/uploads/...`
- `cmd/api/main.go`
  Untuk ubah wiring service, scheduler cleanup, atau inisialisasi storage

## Catatan

- `DATABASE_URL` dan `JWT_SECRET` bisa memakai nilai yang sama seperti `satpam-app`
- Format token JWT dibuat kompatibel secara payload: `userId` dan `role`
- Query login tetap memakai `crypt(..., password_hash)` di PostgreSQL seperti implementasi Next.js

## Migrasi Manual

Untuk kolom offline sync `submit_at`, jalankan SQL berikut:

- [20260313_add_submit_at.sql](C:/Users/agust/OneDrive/Desktop/Kerjaa/Azka/SatpamApi/satpam-go/migrations/20260313_add_submit_at.sql)
- [20260319_add_patrol_attendance_id.sql](C:/Users/agust/OneDrive/Desktop/Kerjaa/Azka/SatpamApi/satpam-go/migrations/20260319_add_patrol_attendance_id.sql)

### Cara menjalankan migrasi

1. Stop server Go dulu jika sedang jalan
2. Pastikan PostgreSQL aktif
3. Jalankan file SQL migrasi ke database yang dipakai `DATABASE_URL`

Contoh dengan `psql` dan value dari `.env.local` saat ini:

```powershell
psql -v ON_ERROR_STOP=1 "postgresql://postgres:root@127.0.0.1:5432/postgres" -f ".\migrations\20260313_add_submit_at.sql"
```

Kalau `psql` belum ada di `PATH`, biasanya bisa pakai path penuh PostgreSQL, contoh:

```powershell
& "C:\Program Files\PostgreSQL\17\bin\psql.exe" -v ON_ERROR_STOP=1 "postgresql://postgres:root@127.0.0.1:5432/postgres" -f ".\migrations\20260313_add_submit_at.sql"
```

Atau kalau mau pakai env di PowerShell:

```powershell
$env:DATABASE_URL="postgresql://postgres:root@127.0.0.1:5432/postgres"
psql -v ON_ERROR_STOP=1 $env:DATABASE_URL -f ".\migrations\20260313_add_submit_at.sql"
```

### Cara cek migrasi sudah masuk

Jalankan query ini:

```powershell
psql "postgresql://postgres:root@127.0.0.1:5432/postgres" -c "\d attendances"
psql "postgresql://postgres:root@127.0.0.1:5432/postgres" -c "\d patrol_scans"
psql "postgresql://postgres:root@127.0.0.1:5432/postgres" -c "\d facility_check_scans"
```

Pastikan masing-masing tabel sudah punya kolom / tabel:

- `submit_at timestamptz`
- `attendance_id uuid` pada `patrol_scans`
- tabel `patrol_runs`

## Progress Patrol Per Shift

Sekarang scan patroli bisa dikaitkan ke `attendance` aktif lewat field opsional `attendanceId` pada `POST /api/v1/patrol/scans`.

- Jika `attendanceId` tidak dikirim, backend akan mencoba mengambil attendance aktif user berdasarkan waktu `scannedAt`
- Backend akan membentuk `patrol_runs` otomatis. Selama ronde aktif belum mencakup semua spot route aktif, scan baru tetap masuk ke ronde yang sama
- Jika semua spot route aktif pada ronde itu sudah terscan, scan berikutnya otomatis membuat ronde baru
- Response `POST /api/v1/patrol/scans` sekarang mengembalikan `patrolRunId`, `patrolRunNo`, `isNewPatrolRun`, dan `patrolRunCompleted`
- `GET /api/v1/patrol/progress?attendanceId=...` akan mengembalikan total spot rute aktif, spot yang sudah dipatroli, spot yang belum dipatroli, jumlah ronde, total scan, dan hitungan scan per spot

Setelah migrasi selesai, jalankan lagi server:

```powershell
air
```
