CREATE TABLE IF NOT EXISTS class_teachers (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    class_id BIGINT UNSIGNED NOT NULL,
    teacher_user_id BIGINT UNSIGNED NOT NULL,
    subject_id BIGINT UNSIGNED NULL,
    role ENUM('homeroom', 'subject_teacher') NOT NULL,
    status ENUM('active', 'removed') NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_class_teachers_class_status (class_id, status),
    KEY idx_class_teachers_teacher_status (teacher_user_id, status),
    CONSTRAINT fk_class_teachers_class FOREIGN KEY (class_id) REFERENCES classes (id) ON DELETE CASCADE,
    CONSTRAINT fk_class_teachers_teacher FOREIGN KEY (teacher_user_id) REFERENCES users (id),
    CONSTRAINT fk_class_teachers_subject FOREIGN KEY (subject_id) REFERENCES subjects (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
