CREATE TABLE IF NOT EXISTS student_profiles (
    user_id BIGINT UNSIGNED NOT NULL,
    nis VARCHAR(50) NOT NULL,
    nisn VARCHAR(50) NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id),
    UNIQUE KEY uq_student_profiles_nis (nis),
    UNIQUE KEY uq_student_profiles_nisn (nisn),
    CONSTRAINT fk_student_profiles_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
