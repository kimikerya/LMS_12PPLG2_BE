CREATE TABLE IF NOT EXISTS staff_profiles (
    user_id BIGINT UNSIGNED NOT NULL,
    employee_id VARCHAR(50) NULL,
    nik VARCHAR(50) NULL,
    nuptk VARCHAR(50) NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id),
    UNIQUE KEY uq_staff_profiles_employee_id (employee_id),
    UNIQUE KEY uq_staff_profiles_nik (nik),
    UNIQUE KEY uq_staff_profiles_nuptk (nuptk),
    CONSTRAINT fk_staff_profiles_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
