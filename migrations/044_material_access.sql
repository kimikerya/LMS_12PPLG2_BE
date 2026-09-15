CREATE TABLE material_access (
 material_id BIGINT UNSIGNED NOT NULL,
 student_user_id BIGINT UNSIGNED NOT NULL,
 first_opened_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 last_opened_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY(material_id,student_user_id),
 CONSTRAINT fk_access_material FOREIGN KEY(material_id) REFERENCES materials(id),
 CONSTRAINT fk_access_student FOREIGN KEY(student_user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
