CREATE TABLE IF NOT EXISTS assessment_answer_options (
    answer_id BIGINT UNSIGNED NOT NULL,
    option_id BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (answer_id, option_id),
    CONSTRAINT fk_assessment_answer_options_answer FOREIGN KEY (answer_id) REFERENCES assessment_answers (id) ON DELETE CASCADE,
    CONSTRAINT fk_assessment_answer_options_option FOREIGN KEY (option_id) REFERENCES question_options (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
