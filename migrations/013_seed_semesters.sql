INSERT IGNORE INTO semesters (academic_year_id, name, start_date, end_date, is_active)
SELECT id, 'ganjil', '2026-07-01', '2026-12-31', TRUE
FROM academic_years WHERE name = '2026/2027';
