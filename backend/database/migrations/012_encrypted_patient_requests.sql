ALTER TABLE patient_appointment_requests
  ADD COLUMN IF NOT EXISTS reason_enc BYTEA;

ALTER TABLE patient_appointment_requests
  DROP CONSTRAINT IF EXISTS patient_appointment_requests_reason_check;

ALTER TABLE patient_appointment_requests
  ADD CONSTRAINT patient_appointment_requests_reason_storage_check
  CHECK (
    length(trim(reason)) BETWEEN 2 AND 300
    OR (reason = '' AND reason_enc IS NOT NULL)
  );
