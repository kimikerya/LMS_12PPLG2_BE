# Endpoint pendukung frontend admin

Semua endpoint berikut membutuhkan Bearer token dengan akun aktif. Pengelolaan
pengguna, edit/hapus kelas, serta penempatan guru tetap khusus admin. Pembuatan
kelas dan referensi akademik juga tersedia untuk guru. Detail kelas hanya tersedia
bagi admin, guru yang ditugaskan, atau siswa yang menjadi anggota aktif.
Middleware memeriksa role/status terbaru dari database. Jalankan `go run ./cmd/migrate`
untuk menerapkan migration 036 (email opsional dan data pribadi) serta 037
(nomor pegawai guru), 038 (penanda akun dihapus), dan 039 (urutan ID akun)
sebelum menjalankan API versi ini.

| Endpoint | Fungsi |
|---|---|
| GET /api/academic-options | Tahun ajaran, jenjang, jurusan aktif, mata pelajaran aktif |
| GET /api/users/{id} | Identitas dan profil NIS/NISN/NIK/NUPTK/nomor pegawai sesuai role |
| POST /api/users | Membuat akun dan profil dalam satu transaksi |
| PATCH /api/users/{id} | Edit identitas/profil; password kosong dipertahankan; role harus tetap |
| PATCH /api/users/{id}/status | Memperbarui status; tidak boleh menonaktifkan akun sendiri |
| POST /api/users/import | Body {"users":[...CreateInput]}; 1–50 akun, seluruh batch atomik |
| POST /api/classes | Membuat kelas dengan referensi akademik yang valid |
| GET /api/classes/{id} | Detail kelas aktif, siswa, penugasan guru, pengumuman |
| PATCH /api/classes/{id} | Edit nama, tingkat, referensi akademik, ruang, deskripsi |
| POST /api/classes/{id}/members | Tambah/keluarkan siswa; endpoint keanggotaan sebelumnya |
| POST /api/classes/{id}/teachers | Tetapkan/hapus penugasan guru; endpoint sebelumnya |
| POST /api/classes/{id}/announcements | Body {"title":"...","content":"..."}; terbitkan teks |
| DELETE /api/users/{id} | Admin: sembunyikan akun dari daftar, blokir login/token, simpan riwayat |
| DELETE /api/classes/{id} | Admin: arsipkan kelas dan nonaktifkan kode, simpan riwayat |
| GET /api/classes/{id}/invite | Admin/guru kelas: kode siswa yang masih aktif |
| POST /api/classes/{id}/invite | Admin/guru kelas: ganti kode acak, masa berlaku 7 hari |
| DELETE /api/classes/{id}/invite | Admin/guru kelas: nonaktifkan semua kode kelas |
| POST /api/classes/join | Siswa: body {"code":"..."}, gabung sebagai siswa saja |

Kolom pengguna: login_id, email, full_name, role, status, password serta profil
nis, nisn, nik, nuptk, employee_id, birth_place, birth_date, phone. Email opsional
disimpan sebagai NULL saat kosong, sehingga beberapa akun boleh tanpa email.
Respons API tetap mengembalikan email kosong sebagai string; login memakai login_id.
Tanggal lahir memakai YYYY-MM-DD (1900 sampai hari ini), tempat lahir maksimal
100 karakter, nomor HP 8–25 karakter. Data pribadi hanya ditambahkan pada detail
pengguna, tidak pada daftar kelas. Nomor pegawai tersedia untuk guru dan staf.
NIK lama tetap kompatibel pada API; formulir FE memakai NIP/nomor pegawai.
Field identitas opsional kosong dinormalisasi
menjadi NULL; NIS wajib untuk siswa. Kata sandi 8–72 byte. Password hash tidak
dikembalikan. Konflik identitas unik menghasilkan 409; validasi menghasilkan 422.
Kesalahan database internal pada operasi baru tidak dikirim ke pengguna.

Import divalidasi dan di-hash sebelum transaksi. Akun dan semua profil ditulis
dalam transaksi yang sama sehingga kegagalan salah satu baris membatalkan batch.
Jika koneksi terputus setelah commit, periksa daftar sebelum mencoba ulang.
Tidak ada idempotency key untuk pembuatan kelas/pengumuman saat ini.

ID login kosong pada pembuatan/import diisi sistem: SIS/GUR/ADM/KUR/KEP diikuti
nomor berurutan minimal 6 digit. Alokasi menggunakan kunci baris dalam transaksi
akun agar permintaan bersamaan tidak memakai nomor yang sama. ID manual tetap
diterima dan diperiksa keunikannya. ID lama tidak diubah atau dipakai ulang.

Kelas wajib berisi title, academic_year_id, education_level_id, major_id dan
grade_level. Description dan room opsional. Guru pembuat kelas juga wajib
mengirim subject_id; penugasan guru mapel dibuat dalam transaksi kelas yang sama.
Guru yang sudah menjadi wali boleh menerima penugasan subject_teacher tambahan.

Siswa dapat membaca ringkasan/guru/pengumuman kelasnya, tanpa daftar identitas
siswa lain. Guru kelas dapat membaca daftar siswanya dan mengelola kode bergabung.
Kode 12 digit heksadesimal dibuat acak; rotasi, penonaktifan, kedaluwarsa, dan arsip
kelas menolak pemakaian kode lama. Gabung berulang tidak membuat anggota ganda;
siswa yang telah dikeluarkan harus ditambahkan kembali oleh admin.

Penghapusan pengguna mengisi deleted_at dan menonaktifkan akun. Penghapusan kelas
mengubah status menjadi archived. Tidak ada hard delete atau UI pemulihan pada
alur ini. Nomor identitas tetap dicadangkan agar riwayat tidak tercampur.
Pengumuman tahap ini belum mendukung edit, hapus atau lampiran.
Waktu pengumuman baru disimpan sebagai UTC dan ditampilkan FE dalam WIB; data
DATETIME lama tidak otomatis dikonversi.

## Pengujian lokal

```powershell
$env:LMS_INTEGRATION = '1'
go test ./...
Remove-Item Env:LMS_INTEGRATION
```

Tes transaksi mencakup rollback import karena NIS duplikat, rollback edit profil,
penolakan perubahan role/nonaktif akun sendiri, referensi kelas, pemisahan
pengumuman antar kelas, dan penolakan tulis pada kelas yang diarsipkan.
Seluruh fixture MySQL tes Go di-rollback.

Tes browser frontend dengan E2E_MANAGEMENT=1 membuat data bertag acak E2E dan
memanggil `go run ./cmd/e2e-cleanup --tag <tag>` setelah selesai. Command hanya
menerima format tag acak yang ditentukan, berjalan pada database lokal, dan
menghapus data sesuai tag beserta audit kelasnya dalam satu transaksi.
