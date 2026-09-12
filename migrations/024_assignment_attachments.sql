CREATE TABLE IF NOT EXISTS assignment_attachments (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    assignment_id BIGINT UNSIGNED NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    file_url VARCHAR(500) NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_assignment_attachments_assignment (assignment_id),
    CONSTRAINT fk_assignment_attachments_assignment FOREIGN KEY (assignment_id) REFERENCES assignments (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
