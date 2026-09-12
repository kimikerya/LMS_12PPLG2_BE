# LMS_12PPLG2_BE

Backend Go EMS SMK Citra Negara dengan MySQL lokal melalui Laragon.

Jalankan dari folder yang berisi `go.mod`:

```powershell
go run ./cmd/migrate
go run ./cmd/api
```

Migration 036 dan 037 menambahkan email opsional, data pribadi pengguna,
serta nomor pegawai guru. Migration 038 dan 039 menambahkan penghapusan dengan
riwayat tetap tersimpan dan penomoran ID login otomatis. Data akun yang sudah ada
tetap dipertahankan.

Panduan API, pengujian otomatis, dan contoh alur tugas:
[docs/BACKEND_TESTING.md](docs/BACKEND_TESTING.md).

Backend masih dalam pengembangan; schema lengkap tidak berarti seluruh API selesai.

Kontrak tambah/edit/import pengguna dan detail kelas admin:
[docs/ADMIN_MANAGEMENT.md](docs/ADMIN_MANAGEMENT.md).
