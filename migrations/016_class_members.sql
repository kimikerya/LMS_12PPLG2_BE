CREATE TABLE IF NOT EXISTS class_members (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    class_id BIGINT UNSIGNED NOT NULL,
    student_user_id BIGINT UNSIGNED NOT NULL,
    joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    joined_via ENUM('admin', 'code', 'invite_link') NOT NULL DEFAULT 'admin',
    status ENUM('active', 'removed') NOT NULL DEFAULT 'active',
    removed_at DATETIME NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_class_members_class_student (class_id, student_user_id),
    KEY idx_class_members_class_status (class_id, status),
    KEY idx_class_members_student_status (student_user_id, status),
    CONSTRAINT fk_class_members_class FOREIGN KEY (class_id) REFERENCES classes (id) ON DELETE CASCADE,
    CONSTRAINT fk_class_members_student FOREIGN KEY (student_user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
