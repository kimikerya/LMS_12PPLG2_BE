CREATE TABLE IF NOT EXISTS semesters (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    academic_year_id BIGINT UNSIGNED NOT NULL,
    name ENUM('ganjil', 'genap') NOT NULL,
    start_date DATE NULL,
    end_date DATE NULL,
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (id),
    UNIQUE KEY uq_semesters_year_name (academic_year_id, name),
    KEY idx_semesters_active (is_active),
    CONSTRAINT fk_semesters_academic_year FOREIGN KEY (academic_year_id) REFERENCES academic_years (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
