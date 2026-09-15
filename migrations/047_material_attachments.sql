CREATE TABLE material_attachments (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 material_id BIGINT UNSIGNED NOT NULL,
 file_name VARCHAR(240) NOT NULL,
 file_url VARCHAR(500) NOT NULL,
 KEY idx_material_attachments_material (material_id),
 CONSTRAINT fk_material_attachment_material FOREIGN KEY (material_id) REFERENCES materials(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
