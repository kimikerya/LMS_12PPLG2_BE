CREATE TABLE IF NOT EXISTS question_options (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    question_id BIGINT UNSIGNED NOT NULL,
    option_text TEXT NOT NULL,
    image_url VARCHAR(500) NULL,
    is_correct BOOLEAN NOT NULL DEFAULT FALSE,
    order_no INT UNSIGNED NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_question_options_order (question_id, order_no),
    KEY idx_question_options_question (question_id),
    CONSTRAINT fk_question_options_question FOREIGN KEY (question_id) REFERENCES assessment_questions (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
