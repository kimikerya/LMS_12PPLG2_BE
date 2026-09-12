# Pemisahan tugas dan asesmen / ulangan

Keputusan domain: **assignment adalah tugas**, bukan ulangan.
**Assessment adalah asesmen / ulangan**, termasuk ulangan harian.

| Konsep | Contoh | API dan tabel utama | Relasi |
|---|---|---|---|
| Tugas (assignment) | PR, latihan, proyek | /api/assignments; assignments | assignment_submissions, submission_files |
| Asesmen / ulangan (assessment) | Kuis, ulangan harian, ujian daring | /api/assessments; assessments | assessment_targets, assessment_questions, assessment_attempts, assessment_answers |

## Alur yang berbeda

- Tugas: guru membuat instruksi dan tenggat; siswa mengumpulkan jawaban;
  guru menilai dan merilis nilai. Backend sekarang mendukung jawaban teks/tautan.
- Ulangan: guru menyiapkan soal, target kelas, jadwal/durasi dan aturan percobaan;
  siswa mengerjakan soal dalam attempt; jawaban dinilai dan hasil dirilis.
  Tabel pendukung sudah ada, tetapi API pengerjaan/timer/penilaian ulangan
  belum lengkap. Adanya tabel tidak berarti seluruh fitur sudah berjalan.
- Nilai tugas tidak otomatis menjadi nilai ulangan. Kedua sumber hasil tetap terpisah.
- Transaction untuk jawaban/nilai/audit bukan transaksi pembayaran.
  Pada alur ulangan berikutnya, gunakan transaction saat menyimpan attempt,
  jawaban, dan perubahan status yang harus berhasil atau gagal bersama.

## Jenis asesmen yang dipertahankan

Schema saat ini memakai assessment_type: quiz atau online_exam.
Label UI quiz adalah "Kuis / Ulangan harian"; online_exam adalah "Ujian daring".
Jangan mengirim "ulangan_harian" sebagai enum baru karena belum didukung database.
Jika kelak perlu membedakan kuis, UH, PTS, PAS sebagai kategori tersendiri,
tentukan kebutuhannya lalu buat migration baru; jangan edit migration yang sudah
pernah dijalankan.

## Dampak ralat istilah

Struktur backend sudah terpisah sebelum ralat, sehingga penyesuaian sekarang
berupa label frontend, penjelasan, dan tes regresi pemetaan endpoint.
Tidak mengganti nama tabel/API, tidak menghapus data, tidak menjalankan migration.
Judul bebas yang berisi kata "ulangan" pada sebuah tugas tidak cukup untuk
memindahkannya otomatis: data lama yang salah tempat harus ditinjau bersama
pemilik data terlebih dahulu.

Frontend memakai /tugas untuk assignments dan /asesmen untuk assessments.
Konfigurasi istilah terpusat di frontend: config/learning.ts.
Jangan memakai alur assignment_submissions untuk pengerjaan soal ulangan.
