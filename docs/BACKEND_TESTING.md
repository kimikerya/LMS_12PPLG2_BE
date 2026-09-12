# Backend: akses kelas dan alur tugas

Backend memakai satu login `POST /auth/login`. Kirim `login_id` dan `password`.
Role berasal dari `users`, lalu diperiksa lagi bersama status akun pada setiap
request. Token milik akun nonaktif ditolak; perubahan role berlaku pada request
berikutnya. Tidak ada public sign-up.

## Menjalankan backend

1. Nyalakan MySQL melalui Laragon.
   Jika muncul `connectex ... 127.0.0.1:3306`, MySQL belum aktif atau tidak
   memakai port 3306. Periksa service MySQL terlebih dahulu; API belum mulai
   mendengarkan port 8080 sebelum koneksi database berhasil.
2. Buka PowerShell di root backend:

   ```powershell
   cd "C:\lms sekolah\LMS_12PPLG2_BE"
   .\scripts\start-api.cmd
   ```

3. Buka `http://localhost:8080/health/db`. Hasil normal: `status: ok`.
4. Root http://localhost:8080/ menampilkan informasi API, bukan tampilan frontend.
   Frontend berjalan di http://localhost:3000/.
5. Hentikan API dengan Ctrl+C pada terminal yang menjalankannya. Jika terminal
   sudah kembali ke prompt tetapi API tetap berjalan, gunakan terminal kedua:

   ```powershell
   cd "C:\lms sekolah\LMS_12PPLG2_BE"
   .\scripts\stop-api.cmd
   ```

   Skrip stop hanya menghentikan executable .local/ems-api.exe milik project ini;
   tidak membunuh semua proses Go atau semua pemakai port 8080. Ia adalah fallback
   penghentian paksa, jadi utamakan Ctrl+C agar request aktif selesai dahulu.
   Skrip tidak menangani executable sementara dari go run lama. Jika port tetap
   terpakai, periksa PID/path pemilik port sebelum menghentikannya.
   Skrip start membangun executable lokal, lalu baru mengumumkan API aktif setelah
   port berhasil dibuka. Tidak menjalankan migration/seed otomatis.
   Pengujian otomatis di bawah memakai httptest, bukan port 8080.

`.env` lokal memakai secret JWT acak. Nilainya jangan dibagikan atau dimasukkan ke
Git. Setelah secret diganti, restart API dan login ulang karena token lama tidak
berlaku. Akun admin/siswa demo yang sudah ada tetap dipertahankan.

## Pengujian otomatis

Unit test (tanpa mewajibkan MySQL):

```powershell
go test ./...
go vet ./...
```

Test integrasi dengan MySQL lokal dan schema migration 001–035 yang sudah ada:

```powershell
$env:LMS_INTEGRATION = '1'
go test ./... -count=1 -v
Remove-Item Env:LMS_INTEGRATION
```

Test integrasi memuat `.env` dari root backend. Ia membuat akun, kelas, dan tugas
sementara di dalam transaction, menjalankan HTTP handler melalui `httptest`, lalu
rollback seluruh fixture. Tidak menjalankan migration atau menghapus tabel. Tidak
menggunakan/mengubah akun ADMIN001 atau STD001. Nomor AUTO_INCREMENT dapat memiliki
celah setelah rollback; itu normal, dan bukan berarti ada data hilang.

Yang diperiksa:

- JWT salah signature, kedaluwarsa, tanpa expiry, algoritme salah, dan role asing ditolak.
- Request tanpa token ditolak; akun nonaktif dan role berubah diperiksa ulang.
- Siswa hanya melihat konten published di kelas aktif yang diikutinya.
- Guru hanya menulis kontennya sendiri pada kelas/mata pelajaran yang ditugaskan.
- Admin, kurikulum, dan kepala sekolah dapat membaca untuk monitoring, tetapi tidak menilai tugas guru.
- Draft dan data kelas lain tidak bocor melalui detail maupun daftar.
- Pengumpulan teks/tautan memeriksa tenggat, status tugas, keanggotaan dan duplikasi.
- Guru pemilik memberi nilai; nilai/feedback disembunyikan dari siswa sebelum rilis.
- Rilis berulang tidak menggandakan notifikasi.
- Kegagalan transaction membatalkan jawaban dan perubahan terkait.
- Batch anggota yang berisi role salah dibatalkan seluruhnya.
- Penugasan wali kelas menghasilkan hanya satu wali aktif, termasuk saat diulang.

## API yang ditambahkan/diperketat

Semua route `/api/*` membutuhkan `Authorization: Bearer TOKEN`.

| Method dan path | Izin / fungsi |
|---|---|
| POST /api/classes/{id}/members | Admin: tambah/aktifkan kembali/keluarkan siswa |
| POST /api/classes/{id}/teachers | Admin: atur guru mata pelajaran atau wali kelas |
| GET /api/materials, /api/assignments, /api/assessments | Daftar sesuai hak akses |
| GET /api/materials/{id}, /api/assignments/{id}, /api/assessments/{id} | Detail sesuai hak akses |
| POST /api/materials, /api/assignments, /api/assessments | Guru membuat draft |
| POST /api/materials/{id}/publish | Guru pemilik menerbitkan materi |
| POST /api/assignments/{id}/publish | Guru pemilik menerbitkan tugas |
| POST /api/assignments/{id}/submissions | Siswa anggota mengumpulkan jawaban |
| GET /api/assignments/{id}/submissions | Siswa: jawaban sendiri; guru pemilik/monitoring: jawaban tugas |
| PATCH /api/submissions/{id}/grade | Guru pemilik memberi nilai |
| POST /api/submissions/{id}/release | Guru pemilik merilis nilai |

Daftar konten menerima `?class_id=ID&limit=50&offset=0` (limit maksimal 100).
Detail menggunakan field `description` untuk deskripsi materi/asesmen atau
instruksi tugas, serta `type` untuk jenis materi/asesmen. Daftar submission saat
ini dibatasi 100 baris per tugas; pagination submission masih pekerjaan berikutnya.

Kode respons: 400 untuk bentuk request salah, 401 untuk sesi tidak sah, 403 untuk
aksi tanpa izin, 404 untuk detail yang tidak dapat diakses, 409 untuk konflik
status/pengiriman ganda, 422 untuk data tidak valid. Error database internal tidak
ditampilkan oleh endpoint learning baru.

## Urutan uji manual melalui API

Gunakan HTTP client pilihan Anda (misalnya terminal PowerShell). Login sebagai
admin memakai akun demo yang sudah ada, kemudian simpan token dari respons.
Berikut contoh pemanggilan PowerShell; variabel ID diisi dari respons API/database,
jangan menganggap nomor ID tertentu selalu sama.

```powershell
$base = 'http://localhost:8080'
$credential = Get-Credential -UserName ADMIN001 -Message 'Password admin lokal'
$loginBody = @{ login_id = $credential.UserName; password = $credential.GetNetworkCredential().Password } | ConvertTo-Json
$login = Invoke-RestMethod "$base/auth/login" -Method Post -ContentType 'application/json' -Body $loginBody
$adminHeaders = @{ Authorization = "Bearer $($login.token)" }
```

1. Admin membuat guru melalui `POST /api/users` jika belum tersedia, dengan role
   `teacher`, login_id/email unik, full_name, dan password. Simpan ID dari respons.
   Guru login melalui endpoint login yang sama. Gunakan token guru untuk langkah
   membuat materi/tugas, bukan token admin.
2. Pilih kelas dari `GET /api/classes` atau buat lewat `POST /api/classes` dengan
   ID tahun ajaran/jenjang/jurusan dari `GET /api/academic-options` (khusus admin).
3. Admin menetapkan siswa ke kelas:

   ```json
   {"user_ids":[123],"status":"active"}
   ```

   Kirim ke `POST /api/classes/{id}/members`. Untuk mengeluarkan anggota gunakan
   `status: removed`; akun pengguna tetap ada. Batch maksimal 100 siswa.
4. Admin menetapkan guru:

   ```json
   {"teacher_user_id":456,"subject_id":1,"role":"subject_teacher"}
   ```

   Kirim ke `POST /api/classes/{id}/teachers`. Untuk wali gunakan role `homeroom`.
   Mengaktifkan wali baru menonaktifkan penugasan wali lama dalam transaction.
5. Guru membuat tugas melalui `POST /api/assignments`:

   ```json
   {"class_id":1,"subject_id":1,"title":"Latihan fungsi","instructions":"Jelaskan fungsi pada Go","due_at":"2026-12-31T15:00:00+07:00","allow_late":false,"max_points":100}
   ```

   Ganti semua ID dan tanggal sesuai data uji Anda. Nilai default max_points adalah
   100. Waktu API memakai RFC3339 dengan zona waktu, disimpan sebagai UTC. Data
   DATETIME lama tidak dikonversi otomatis; tinjau zona waktunya sebelum import.
6. Guru memanggil `POST /api/assignments/{id}/publish`. Siswa belum dapat melihat draft.
7. Siswa login, lalu mengirim ke `POST /api/assignments/{id}/submissions`:

   ```json
   {"submission_type":"text","text_answer":"Fungsi mengelompokkan instruksi yang dapat dipanggil kembali."}
   ```

   Alternatif: `submission_type: link` dan `link_url` HTTP/HTTPS. Satu kali submit
   per siswa/tugas; edit atau kirim ulang belum tersedia. Jika `allow_late=true`,
   jawaban setelah due_at berstatus late; close_at tetap menjadi batas mutlak.
8. Guru memberi nilai lewat `PATCH /api/submissions/{id}/grade`:

   ```json
   {"score":85,"teacher_feedback":"Jawaban sudah sesuai."}
   ```

   Score tidak boleh melebihi max_points. Siswa masih menerima score/feedback null.
9. Guru memanggil `POST /api/submissions/{id}/release`. Siswa dapat melihat nilai
   melalui `GET /api/assignments/{id}/submissions`. Rilis juga membuat notifikasi
   dan audit log secara atomik. Koreksi nilai yang sudah dirilis belum tersedia.

## Batas pengerjaan sekarang

Pemetaan istilah dan cakupan ulangan dijelaskan di [DOMAIN_LEARNING.md](DOMAIN_LEARNING.md).
Assignment = tugas; assessment = asesmen/ulangan, termasuk ulangan harian.

Ini menyelesaikan satu alur tugas berbasis teks/tautan dan pengaturan aksesnya.
Upload/download file, soal dan pengerjaan asesmen, penilaian ujian, pengumuman,
API notifikasi/laporan, edit profil lengkap, logout/revokasi sesi dan pembatasan
percobaan login belum lengkap. Jangan menyebut seluruh backend siap produksi.
Frontend dapat mulai menguji endpoint yang sudah tersedia, tetapi integrasi penuh
masih memerlukan penyelesaian fitur tersebut.

Tidak ada perubahan schema dalam pekerjaan ini. Migration lama tetap dipertahankan.
Transaction DML melindungi data seperti users/profile, jawaban, nilai, dan audit.
MySQL CREATE/ALTER TABLE menyebabkan implicit commit, sehingga rollback biasa
tidak membatalkan perubahan struktur. Bila migration gagal, periksa tabel dan
schema_migrations sebelum menjalankan ulang; buat migration lanjutan untuk schema
yang sudah pernah digunakan. Rujukan:
https://dev.mysql.com/doc/refman/8.0/en/implicit-commit.html
