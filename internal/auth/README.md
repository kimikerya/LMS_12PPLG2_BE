# Login identitas sekolah

Login hanya menerima NIS siswa atau NIP/nomor pegawai guru, kurikulum, kepala sekolah, dan admin. `users.login_id` tetap menjadi kode internal/metadata JWT, bukan kredensial login. Field JSON request tetap bernama `login_id` untuk kompatibilitas, tetapi nilainya harus identitas sekolah.

Identitas yang ditemukan pada lebih dari satu akun tidak dipilih secara acak: login ditolak. Akun tanpa NIS/NIP harus dilengkapi admin sebelum dapat login. NISN, NUPTK, email, dan nama bukan identitas login. Perubahan identitas login tidak mengganti password akun.

Tes MySQL: jalankan `$env:LMS_INTEGRATION='1'; go test ./...` di PowerShell. Fixture di-rollback setelah tes.

Pembaruan keamanan: jalankan migrasi `046_user_auth_version.sql` sebelum memakai
backend terbaru. Reset password dan perubahan status membatalkan sesi sebelumnya.
Login dibatasi 10 percobaan per identitas/akun per menit pada satu proses API;
respons 429 meminta pengguna menunggu, tanpa mengubah data akunnya.
