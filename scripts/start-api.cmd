@echo off
setlocal
pushd "%~dp0.."
echo Memeriksa MySQL pada 127.0.0.1:3306...
powershell.exe -NoProfile -Command "$ok = Test-NetConnection -ComputerName 127.0.0.1 -Port 3306 -InformationLevel Quiet; if (-not $ok) { Write-Output 'MySQL belum aktif pada 127.0.0.1:3306.'; Write-Output 'Nyalakan MySQL melalui Laragon/XAMPP, lalu jalankan script ini lagi.'; exit 1 }"
if errorlevel 1 (
  popd
  exit /b 1
)
if not exist ".local" mkdir ".local"
go build -o .local\ems-api.exe ./cmd/api
if errorlevel 1 (
  echo Build gagal. Jika API masih berjalan, jalankan scripts\stop-api.cmd terlebih dahulu.
  popd
  exit /b 1
)
echo API berjalan di http://127.0.0.1:8080. Tekan Ctrl+C untuk menghentikan API secara normal.
".local\ems-api.exe"
set "apiExit=%errorlevel%"
popd
exit /b %apiExit%
