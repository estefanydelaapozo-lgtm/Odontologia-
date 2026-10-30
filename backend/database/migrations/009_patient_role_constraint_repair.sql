-- Reparación idempotente para instalaciones que ya registraron una migración
-- previa pero conservan una restricción users_role_check sin el rol paciente.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users
  ADD CONSTRAINT users_role_check
  CHECK (role IN ('admin','recepcionista','odontologo','asistente','auditor','paciente'));
