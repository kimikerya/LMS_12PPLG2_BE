-- Kode bergabung dibuat sekali per kelas dan tidak kedaluwarsa.
UPDATE class_invites
SET expires_at = NULL
WHERE is_active = TRUE;
