@echo off
setlocal
echo Mencari API backend milik project ini...
set "EMS_API_ROOT=%~dp0.."
powershell.exe -NoProfile -Command "$target = [IO.Path]::GetFullPath((Join-Path $env:EMS_API_ROOT '.local\ems-api.exe')); $processes = @(Get-Process -Name ems-api -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $target }); if ($processes.Count -eq 0) { Write-Output 'Tidak ada proses API project ini yang berjalan.' } else { $processes | Stop-Process -ErrorAction Stop; Write-Output 'API project ini dihentikan.' }"
exit /b %errorlevel%
