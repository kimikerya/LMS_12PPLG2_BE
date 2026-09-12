ALTER TABLE teacher_profiles
 ADD COLUMN employee_id VARCHAR(50) NULL,
 ADD UNIQUE KEY uq_teacher_profiles_employee_id (employee_id);
