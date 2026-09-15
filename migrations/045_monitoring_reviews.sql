CREATE TABLE monitoring_reviews (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 target_user_id BIGINT UNSIGNED NOT NULL,
 class_id BIGINT UNSIGNED NOT NULL,
 subject_id BIGINT UNSIGNED NOT NULL,
 author_user_id BIGINT UNSIGNED NOT NULL,
 status ENUM('review','monitor','resolved') NOT NULL,
 note TEXT NOT NULL,
 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 KEY idx_review_target(target_user_id,class_id,subject_id,created_at),
 CONSTRAINT fk_review_target FOREIGN KEY(target_user_id) REFERENCES users(id),
 CONSTRAINT fk_review_class FOREIGN KEY(class_id) REFERENCES classes(id),
 CONSTRAINT fk_review_subject FOREIGN KEY(subject_id) REFERENCES subjects(id),
 CONSTRAINT fk_review_author FOREIGN KEY(author_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
