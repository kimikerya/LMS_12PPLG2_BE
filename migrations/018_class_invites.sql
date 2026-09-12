CREATE TABLE IF NOT EXISTS class_invites (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    class_id BIGINT UNSIGNED NOT NULL,
    code VARCHAR(50) NULL,
    token VARCHAR(255) NULL,
    created_by BIGINT UNSIGNED NOT NULL,
    expires_at DATETIME NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_class_invites_code (code),
    UNIQUE KEY uq_class_invites_token (token),
    KEY idx_class_invites_class_active (class_id, is_active),
    CONSTRAINT fk_class_invites_class FOREIGN KEY (class_id) REFERENCES classes (id) ON DELETE CASCADE,
    CONSTRAINT fk_class_invites_creator FOREIGN KEY (created_by) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
