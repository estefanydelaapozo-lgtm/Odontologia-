CREATE TABLE IF NOT EXISTS patient_appointment_requests (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id),
  preferred_at TIMESTAMPTZ NOT NULL,
  reason TEXT NOT NULL CHECK (length(trim(reason)) BETWEEN 2 AND 300),
  status TEXT NOT NULL DEFAULT 'pendiente' CHECK (status IN ('pendiente','confirmada','rechazada')),
  reception_note TEXT NOT NULL DEFAULT '',
  reviewed_by BIGINT REFERENCES users(id),
  reviewed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_patient_requests_status_date ON patient_appointment_requests(status, preferred_at);
CREATE INDEX IF NOT EXISTS idx_patient_requests_user_date ON patient_appointment_requests(user_id, created_at DESC);
