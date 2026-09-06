CREATE TABLE IF NOT EXISTS teacher_profiles (
    user_id BIGINT UNSIGNED NOT NULL,
    nik VARCHAR(50) NULL,
    nuptk VARCHAR(50) NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id),
    UNIQUE KEY uq_teacher_profiles_nik (nik),
    UNIQUE KEY uq_teacher_profiles_nuptk (nuptk),
    CONSTRAINT fk_teacher_profiles_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
