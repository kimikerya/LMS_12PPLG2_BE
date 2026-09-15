# Kebijakan kata sandi

Minimal 5 karakter Unicode (code point), maksimal 72 byte UTF-8 sesuai batas bcrypt. Dipakai bersama oleh validasi tambah/edit/impor pengguna dan seed admin/siswa. Password kosong saat edit tidak mengganti password lama. Login tetap memverifikasi hash, tanpa memaksakan batas minimum baru pada akun lama.

Tidak memerlukan migration atau seed ulang; password akun yang ada tidak berubah. Restart API agar aturan baru digunakan. Minimum 5 karakter lebih lemah untuk penggunaan publik; gunakan password lebih panjang bila memungkinkan.
