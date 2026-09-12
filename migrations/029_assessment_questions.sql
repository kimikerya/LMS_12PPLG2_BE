CREATE TABLE IF NOT EXISTS assessment_questions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    assessment_id BIGINT UNSIGNED NOT NULL,
    question_type ENUM('single_choice', 'multiple_choice', 'essay') NOT NULL,
    question_text TEXT NOT NULL,
    image_url VARCHAR(500) NULL,
    points DECIMAL(6,2) NOT NULL,
    order_no INT UNSIGNED NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_assessment_questions_order (assessment_id, order_no),
    CONSTRAINT fk_assessment_questions_assessment FOREIGN KEY (assessment_id) REFERENCES assessments (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
