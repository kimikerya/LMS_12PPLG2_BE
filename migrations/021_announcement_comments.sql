CREATE TABLE IF NOT EXISTS announcement_comments (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    announcement_id BIGINT UNSIGNED NOT NULL,
    author_user_id BIGINT UNSIGNED NOT NULL,
    content TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at DATETIME NULL,
    PRIMARY KEY (id),
    KEY idx_announcement_comments_announcement_created (announcement_id, created_at),
    KEY idx_announcement_comments_author (author_user_id),
    CONSTRAINT fk_announcement_comments_announcement FOREIGN KEY (announcement_id) REFERENCES announcements (id) ON DELETE CASCADE,
    CONSTRAINT fk_announcement_comments_author FOREIGN KEY (author_user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
