CREATE TABLE IF NOT EXISTS assessment_answers (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    attempt_id BIGINT UNSIGNED NOT NULL,
    question_id BIGINT UNSIGNED NOT NULL,
    essay_text TEXT NULL,
    awarded_points DECIMAL(6,2) NULL,
    teacher_feedback TEXT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_assessment_answers_attempt_question (attempt_id, question_id),
    CONSTRAINT fk_assessment_answers_attempt FOREIGN KEY (attempt_id) REFERENCES assessment_attempts (id) ON DELETE CASCADE,
    CONSTRAINT fk_assessment_answers_question FOREIGN KEY (question_id) REFERENCES assessment_questions (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
