CREATE TABLE IF NOT EXISTS announcement_attachments (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    announcement_id BIGINT UNSIGNED NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    file_url VARCHAR(500) NOT NULL,
    file_type VARCHAR(100) NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_announcement_attachments_announcement (announcement_id),
    CONSTRAINT fk_announcement_attachments_announcement FOREIGN KEY (announcement_id) REFERENCES announcements (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
