# Backup dan pemulihan LMS

Jalankan dari folder backend. MySQL harus aktif. Alat memakai `.env` tanpa
menyalin password atau JWT secret ke backup. Simpan konfigurasi rahasia secara
terpisah di tempat yang terlindungi.

## Membuat backup

```powershell
go run ./cmd/backup
```

Hasil berada di `.local/backups/backup-...`: `database.sql`, folder `uploads`,
dan `manifest.json` berisi checksum setiap file. Folder `.local` tidak masuk Git.
Salin satu folder backup lengkap ke penyimpanan lain yang aksesnya dibatasi.
Backup berisi data pribadi, jawaban, dan hash password akun; jangan unggah ke repo.

Jika MySQL tidak ditemukan otomatis:

```powershell
go run ./cmd/backup -mysql-bin "C:\laragon\bin\mysql\VERSI-MYSQL\bin"
```

Backup menggunakan snapshot transaksi MySQL dan menyalin file unggahan.
Jangan menjalankan migrasi, penghapusan file manual, atau pembersihan storage
selama proses ini. Untuk backup menjelang UKK, hentikan API dengan Ctrl+C dahulu
agar database dan unggahan tidak berubah, lalu jalankan kembali setelah selesai.
Backup tidak menghentikan server sendiri dan tidak menghapus backup lama.

## Memeriksa dan memulihkan

Ganti `PATH-BACKUP` dengan folder hasil backup:

```powershell
go run ./cmd/backup -mode verify -source "PATH-BACKUP"
go run ./cmd/backup -mode restore -source "PATH-BACKUP" -target-db lms_restore_ukk
```

Restore menolak database tujuan yang sudah ada atau sama dengan `DB_NAME`.
File disalin ke folder baru di `.local/restores`. Konfigurasi aplikasi aktif
tidak diubah. Jika gagal, database/folder hasil sebagian dibiarkan untuk ditinjau;
gunakan nama database baru saat mencoba lagi.

Setelah restore, uji di salinan backend terpisah dengan `DB_NAME` menunjuk database
hasil restore, `LMS_UPLOAD_DIR` menunjuk folder hasil restore, dan port berbeda.
Cek login, kelas, nilai, serta unduhan lampiran sebelum menjadikan hasil restore
sebagai database aktif. Hanya gunakan backup tepercaya; checksum memeriksa
kerusakan file, bukan membuktikan sumber SQL aman.

## Rutinitas server

- Backup sebelum perubahan besar dan setelah input data penting; saat dipakai
  rutin, jalankan setiap hari melalui Task Scheduler dengan working directory BE.
- Simpan salinan di lokasi lain, jangan hanya di disk server yang sama.
- Periksa backup dan coba restore berkala. Ukur ruang disk untuk backup/unggahan.
- `GET /health/db` mengembalikan 200 jika database terhubung, atau 503 jika gagal.
- Jalankan FE dengan build produksi; simpan log server dan tinjau request lambat.
- Kapasitas pengguna serentak tetap perlu diuji pada server hosting tujuan.
