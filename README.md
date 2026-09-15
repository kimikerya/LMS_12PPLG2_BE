# LMS SMK Citra Negara — Backend

API Go dan MySQL untuk akun, kelas, materi, tugas, asesmen, serta laporan LMS.
Frontend: [LMS_12PPLG2_FE](https://github.com/kimikerya/LMS_12PPLG2_FE).

## Instalasi

Siapkan Go 1.26.8 atau lebih baru dan MySQL. Dari folder backend:

```powershell
Copy-Item .env.example .env
go mod download
```

Isi koneksi database dan `AUTH_JWT_SECRET` acak minimal 32 byte pada `.env`.
Buat database sesuai `DB_NAME`, lalu jalankan:

```powershell
go run ./cmd/migrate
.\scripts\start-api.cmd
```

API tersedia di http://127.0.0.1:8080. Periksa koneksi database melalui
`/health/db`. Hentikan server dengan `Ctrl+C`.
Untuk sistem selain Windows, jalankan `go run ./cmd/api`.
Jangan menimpa `.env` yang sudah dikonfigurasi atau memasukkannya ke Git.

## Build

```powershell
go build -o .local/api.exe ./cmd/api
```

## Penyimpanan dan backup

File materi, tugas, dan jawaban disimpan di `.local/uploads` atau lokasi
`LMS_UPLOAD_DIR`. Database dan file unggahan harus dicadangkan bersama.

```powershell
go run ./cmd/backup
```

Petunjuk verifikasi dan pemulihan tersedia di [panduan backup](scripts/BACKUP.md).
Konfigurasi rahasia, backup, dan unggahan tidak disertakan dalam repo.
