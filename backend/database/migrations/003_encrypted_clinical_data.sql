ALTER TABLE dental_charts ADD COLUMN IF NOT EXISTS chart_data_enc BYTEA;
ALTER TABLE evolution_notes ADD COLUMN IF NOT EXISTS subjective_enc BYTEA;
ALTER TABLE evolution_notes ADD COLUMN IF NOT EXISTS objective_enc BYTEA;
ALTER TABLE evolution_notes ADD COLUMN IF NOT EXISTS evaluation_enc BYTEA;
ALTER TABLE evolution_notes ADD COLUMN IF NOT EXISTS plan_enc BYTEA;
ALTER TABLE patient_consents ADD COLUMN IF NOT EXISTS snapshot_enc BYTEA;
ALTER TABLE patient_consents ADD COLUMN IF NOT EXISTS signature_data_enc BYTEA;
ALTER TABLE treatment_performances ADD COLUMN IF NOT EXISTS notes_enc BYTEA;
ALTER TABLE clinical_addenda ADD COLUMN IF NOT EXISTS correction_enc BYTEA;
ALTER TABLE notification_outbox ADD COLUMN IF NOT EXISTS payload_enc BYTEA;
ALTER TABLE users ADD COLUMN IF NOT EXISTS signature_public_key BYTEA;
ALTER TABLE users ADD COLUMN IF NOT EXISTS signature_private_key_enc BYTEA;
CREATE TABLE IF NOT EXISTS patient_periodontograms (
  id BIGSERIAL PRIMARY KEY, patient_id BIGINT NOT NULL REFERENCES patients(id), measurements_enc BYTEA NOT NULL,
  recorded_by BIGINT NOT NULL REFERENCES users(id), measured_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_patient_periodontograms_patient_date ON patient_periodontograms(patient_id,measured_at DESC);
CREATE OR REPLACE FUNCTION forbid_addendum_mutation() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'Una adenda clínica es inmutable'; END; $$ LANGUAGE plpgsql;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='clinical_addenda_immutable') THEN
    EXECUTE 'CREATE TRIGGER clinical_addenda_immutable BEFORE UPDATE OR DELETE ON clinical_addenda FOR EACH ROW EXECUTE FUNCTION forbid_addendum_mutation()';
  END IF;
END $$;
