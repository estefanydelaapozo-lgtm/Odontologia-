-- Allow the patient portal role used by local demonstration accounts.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check
  CHECK (role IN ('admin','recepcionista','odontologo','asistente','auditor','paciente'));
