CREATE TABLE IF NOT EXISTS assessment_targets (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    assessment_id BIGINT UNSIGNED NOT NULL,
    class_id BIGINT UNSIGNED NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_assessment_targets_assessment_class (assessment_id, class_id),
    KEY idx_assessment_targets_class (class_id),
    CONSTRAINT fk_assessment_targets_assessment FOREIGN KEY (assessment_id) REFERENCES assessments (id) ON DELETE CASCADE,
    CONSTRAINT fk_assessment_targets_class FOREIGN KEY (class_id) REFERENCES classes (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
