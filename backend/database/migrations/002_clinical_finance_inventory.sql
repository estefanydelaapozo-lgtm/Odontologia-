ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_secret_ciphertext BYTEA;
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_setup_secret_enc BYTEA;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;
ALTER TABLE patients ADD COLUMN IF NOT EXISTS insurance_policy TEXT NOT NULL DEFAULT '';
ALTER TABLE patients ADD COLUMN IF NOT EXISTS consent_for_contact BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE patients ADD COLUMN IF NOT EXISTS medical_alerts_enc BYTEA;
ALTER TABLE patients ADD COLUMN IF NOT EXISTS allergies_enc BYTEA;
ALTER TABLE patients ADD COLUMN IF NOT EXISTS medical_history_enc BYTEA;
ALTER TABLE chairs ADD COLUMN IF NOT EXISTS room_code TEXT NOT NULL DEFAULT '';
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS treatment_item_id BIGINT;
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS reminder_48h_sent_at TIMESTAMPTZ;
ALTER TABLE appointments ADD COLUMN IF NOT EXISTS reminder_24h_sent_at TIMESTAMPTZ;
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS fiscal_status TEXT NOT NULL DEFAULT 'no_solicitado';
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS fiscal_reference TEXT NOT NULL DEFAULT '';
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS fiscal_response JSONB;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS insurance_claim TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS clinics (
  id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, tax_id TEXT NOT NULL DEFAULT '', timezone TEXT NOT NULL DEFAULT 'America/Guayaquil', active BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE TABLE IF NOT EXISTS rooms (
  id BIGSERIAL PRIMARY KEY, clinic_id BIGINT REFERENCES clinics(id), code TEXT NOT NULL, name TEXT NOT NULL, active BOOLEAN NOT NULL DEFAULT TRUE, UNIQUE(clinic_id,code)
);
CREATE TABLE IF NOT EXISTS appointment_settings (
  id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id=1), note_lock_hours INTEGER NOT NULL DEFAULT 24 CHECK (note_lock_hours>0), timezone TEXT NOT NULL DEFAULT 'America/Guayaquil', updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO appointment_settings(id) VALUES(1) ON CONFLICT(id) DO NOTHING;

CREATE TABLE IF NOT EXISTS dental_charts (
  id BIGSERIAL PRIMARY KEY, patient_id BIGINT NOT NULL REFERENCES patients(id), dentition TEXT NOT NULL CHECK(dentition IN ('permanente','temporal','mixta')),
  notation TEXT NOT NULL CHECK(notation IN ('FDI','UNIVERSAL')), chart_data JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by BIGINT NOT NULL REFERENCES users(id), updated_by BIGINT NOT NULL REFERENCES users(id), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(patient_id,dentition,notation)
);
CREATE TABLE IF NOT EXISTS tooth_findings (
  id BIGSERIAL PRIMARY KEY, chart_id BIGINT NOT NULL REFERENCES dental_charts(id), tooth_code TEXT NOT NULL,
  condition TEXT NOT NULL CHECK(condition IN ('caries','restauracion','endodoncia_realizada','endodoncia_requerida','implante','protesis_fija','protesis_removible','exodoncia','sellante','sano','otro')),
  surfaces JSONB NOT NULL DEFAULT '[]'::jsonb, notes TEXT NOT NULL DEFAULT '', color TEXT NOT NULL DEFAULT '', recorded_by BIGINT NOT NULL REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tooth_findings_chart_tooth ON tooth_findings(chart_id,tooth_code);
CREATE TABLE IF NOT EXISTS periodontal_measurements (
  id BIGSERIAL PRIMARY KEY, patient_id BIGINT NOT NULL REFERENCES patients(id), tooth_code TEXT NOT NULL,
  site TEXT NOT NULL CHECK(site IN ('mesiovestibular','vestibular','distovestibular','mesiolingual','lingual','distolingual')),
  probing_depth_mm NUMERIC(4,1) NOT NULL CHECK(probing_depth_mm BETWEEN 0 AND 20), gingival_margin_mm NUMERIC(4,1) NOT NULL DEFAULT 0,
  clinical_attachment_level_mm NUMERIC(4,1) NOT NULL, furcation TEXT NOT NULL DEFAULT 'ninguna', mobility SMALLINT NOT NULL DEFAULT 0 CHECK(mobility BETWEEN 0 AND 3),
  bleeding BOOLEAN NOT NULL DEFAULT FALSE, recorded_by BIGINT NOT NULL REFERENCES users(id), measured_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_periodontal_patient_date ON periodontal_measurements(patient_id,measured_at DESC);

CREATE TABLE IF NOT EXISTS evolution_notes (
  id BIGSERIAL PRIMARY KEY, patient_id BIGINT NOT NULL REFERENCES patients(id), appointment_id BIGINT REFERENCES appointments(id),
  subjective TEXT NOT NULL, objective TEXT NOT NULL, evaluation TEXT NOT NULL, plan TEXT NOT NULL,
  author_id BIGINT NOT NULL REFERENCES users(id), signed_at TIMESTAMPTZ, digital_signature TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE OR REPLACE FUNCTION guard_evolution_note() RETURNS trigger AS $$
DECLARE lock_hours INTEGER;
BEGIN
  SELECT note_lock_hours INTO lock_hours FROM appointment_settings WHERE id=1;
  IF OLD.signed_at IS NOT NULL OR now() > OLD.created_at + make_interval(hours => COALESCE(lock_hours,24)) THEN
    RAISE EXCEPTION 'La nota de evolución está bloqueada; registre una adenda firmada';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='evolution_notes_lock') THEN
    EXECUTE 'CREATE TRIGGER evolution_notes_lock BEFORE UPDATE OR DELETE ON evolution_notes FOR EACH ROW EXECUTE FUNCTION guard_evolution_note()';
  END IF;
END $$;
CREATE TABLE IF NOT EXISTS clinical_addenda (
  id BIGSERIAL PRIMARY KEY, patient_id BIGINT NOT NULL REFERENCES patients(id), source_note_id BIGINT REFERENCES evolution_notes(id),
  correction TEXT NOT NULL, signed_by BIGINT NOT NULL REFERENCES users(id), digital_signature TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS consent_templates (
  id BIGSERIAL PRIMARY KEY, code TEXT NOT NULL UNIQUE, procedure_name TEXT NOT NULL, version TEXT NOT NULL, body TEXT NOT NULL, active BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE TABLE IF NOT EXISTS patient_consents (
  id BIGSERIAL PRIMARY KEY, patient_id BIGINT NOT NULL REFERENCES patients(id), template_id BIGINT NOT NULL REFERENCES consent_templates(id),
  template_version TEXT NOT NULL, snapshot TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pendiente' CHECK(status IN ('pendiente','firmado','vencido','revocado')),
  signature_method TEXT CHECK(signature_method IN ('biometrica','otp')), signature_data TEXT NOT NULL DEFAULT '', otp_hash TEXT NOT NULL DEFAULT '', otp_expires_at TIMESTAMPTZ,
  signed_at TIMESTAMPTZ, created_by BIGINT NOT NULL REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS consent_otp_attempts (
  id BIGSERIAL PRIMARY KEY, consent_id BIGINT NOT NULL REFERENCES patient_consents(id), otp_hash TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL,
  attempts SMALLINT NOT NULL DEFAULT 0, consumed_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS treatment_plans (
  id BIGSERIAL PRIMARY KEY, patient_id BIGINT NOT NULL REFERENCES patients(id), title TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'borrador' CHECK(status IN ('borrador','presentado','aprobado_parcial','aprobado','rechazado','completado')),
  created_by BIGINT NOT NULL REFERENCES users(id), approved_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS treatment_plan_items (
  id BIGSERIAL PRIMARY KEY, plan_id BIGINT NOT NULL REFERENCES treatment_plans(id), service_id BIGINT NOT NULL REFERENCES services(id),
  phase_name TEXT NOT NULL DEFAULT 'Fase 1', session_number INTEGER NOT NULL DEFAULT 1 CHECK(session_number>0), description TEXT NOT NULL DEFAULT '',
  professional_fee NUMERIC(12,2) NOT NULL CHECK(professional_fee>=0), laboratory_cost NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK(laboratory_cost>=0),
  discount NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK(discount>=0), discount_authorized_by BIGINT REFERENCES users(id), quantity INTEGER NOT NULL DEFAULT 1 CHECK(quantity>0),
  approved_quantity INTEGER NOT NULL DEFAULT 0 CHECK(approved_quantity>=0), status TEXT NOT NULL DEFAULT 'propuesto' CHECK(status IN ('propuesto','aprobado','agendado','realizado','rechazado'))
);
ALTER TABLE appointments ADD CONSTRAINT fk_appointments_treatment_item FOREIGN KEY(treatment_item_id) REFERENCES treatment_plan_items(id);
CREATE TABLE IF NOT EXISTS treatment_approvals (
  id BIGSERIAL PRIMARY KEY, plan_id BIGINT NOT NULL REFERENCES treatment_plans(id), patient_id BIGINT NOT NULL REFERENCES patients(id),
  approval_type TEXT NOT NULL CHECK(approval_type IN ('total','parcial','rechazo')), approved_item_ids BIGINT[] NOT NULL DEFAULT '{}',
  approval_method TEXT NOT NULL CHECK(approval_method IN ('presencial','otp')), signature_data TEXT NOT NULL DEFAULT '', approved_at TIMESTAMPTZ NOT NULL DEFAULT now(), recorded_by BIGINT NOT NULL REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS notification_outbox (
  id BIGSERIAL PRIMARY KEY, patient_id BIGINT REFERENCES patients(id), appointment_id BIGINT REFERENCES appointments(id),
  channel TEXT NOT NULL CHECK(channel IN ('whatsapp','sms','email')), destination TEXT NOT NULL, template TEXT NOT NULL, payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  scheduled_at TIMESTAMPTZ NOT NULL, sent_at TIMESTAMPTZ, status TEXT NOT NULL DEFAULT 'pendiente' CHECK(status IN ('pendiente','enviado','fallido')),
  provider_response TEXT NOT NULL DEFAULT '', idempotency_key TEXT NOT NULL UNIQUE, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_notification_outbox_pending ON notification_outbox(status,scheduled_at);

CREATE TABLE IF NOT EXISTS invoice_items (
  id BIGSERIAL PRIMARY KEY, invoice_id BIGINT NOT NULL REFERENCES invoices(id), description TEXT NOT NULL, quantity NUMERIC(10,2) NOT NULL CHECK(quantity>0),
  unit_price NUMERIC(12,2) NOT NULL CHECK(unit_price>=0), discount NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK(discount>=0), total NUMERIC(12,2) NOT NULL CHECK(total>=0),
  service_id BIGINT REFERENCES services(id), treatment_item_id BIGINT REFERENCES treatment_plan_items(id)
);
CREATE TABLE IF NOT EXISTS fiscal_jobs (
  id BIGSERIAL PRIMARY KEY, invoice_id BIGINT NOT NULL REFERENCES invoices(id), provider TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pendiente',
  request_payload JSONB NOT NULL, response_payload JSONB, provider_reference TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS professional_fee_rules (
  id BIGSERIAL PRIMARY KEY, professional_id BIGINT NOT NULL REFERENCES professionals(id), service_id BIGINT REFERENCES services(id),
  rule_type TEXT NOT NULL CHECK(rule_type IN ('porcentaje','fijo','porcentaje_neto')), rate NUMERIC(8,4) NOT NULL CHECK(rate>=0), active BOOLEAN NOT NULL DEFAULT TRUE, valid_from DATE NOT NULL DEFAULT CURRENT_DATE
);
CREATE TABLE IF NOT EXISTS professional_settlements (
  id BIGSERIAL PRIMARY KEY, professional_id BIGINT NOT NULL REFERENCES professionals(id), period_start DATE NOT NULL, period_end DATE NOT NULL,
  gross NUMERIC(12,2) NOT NULL DEFAULT 0, deductions NUMERIC(12,2) NOT NULL DEFAULT 0, net NUMERIC(12,2) NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pendiente' CHECK(status IN ('pendiente','aprobada','pagada')), created_by BIGINT NOT NULL REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS professional_settlement_items (
  id BIGSERIAL PRIMARY KEY, settlement_id BIGINT NOT NULL REFERENCES professional_settlements(id), treatment_id BIGINT NOT NULL,
  service_id BIGINT REFERENCES services(id), gross NUMERIC(12,2) NOT NULL, deductions NUMERIC(12,2) NOT NULL DEFAULT 0, net NUMERIC(12,2) NOT NULL
);

CREATE TABLE IF NOT EXISTS inventory_items (
  id BIGSERIAL PRIMARY KEY, sku TEXT NOT NULL UNIQUE, name TEXT NOT NULL, unit TEXT NOT NULL DEFAULT 'unidad', minimum_stock NUMERIC(12,3) NOT NULL DEFAULT 0,
  active BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE TABLE IF NOT EXISTS inventory_balances (
  id BIGSERIAL PRIMARY KEY, item_id BIGINT NOT NULL REFERENCES inventory_items(id), chair_id BIGINT REFERENCES chairs(id), location TEXT NOT NULL DEFAULT '',
  quantity NUMERIC(12,3) NOT NULL DEFAULT 0 CHECK(quantity>=0), UNIQUE(item_id,chair_id,location)
);
CREATE TABLE IF NOT EXISTS service_materials (
  id BIGSERIAL PRIMARY KEY, service_id BIGINT NOT NULL REFERENCES services(id), item_id BIGINT NOT NULL REFERENCES inventory_items(id), quantity NUMERIC(12,3) NOT NULL CHECK(quantity>0), UNIQUE(service_id,item_id)
);
CREATE TABLE IF NOT EXISTS inventory_movements (
  id BIGSERIAL PRIMARY KEY, item_id BIGINT NOT NULL REFERENCES inventory_items(id), chair_id BIGINT REFERENCES chairs(id),
  movement_type TEXT NOT NULL CHECK(movement_type IN ('entrada','consumo','ajuste','devolucion')), quantity NUMERIC(12,3) NOT NULL CHECK(quantity>0),
  reference_type TEXT NOT NULL, reference_id BIGINT NOT NULL, recorded_by BIGINT NOT NULL REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS treatment_performances (
  id BIGSERIAL PRIMARY KEY, patient_id BIGINT NOT NULL REFERENCES patients(id), plan_item_id BIGINT REFERENCES treatment_plan_items(id), appointment_id BIGINT REFERENCES appointments(id),
  service_id BIGINT NOT NULL REFERENCES services(id), professional_id BIGINT NOT NULL REFERENCES professionals(id), performed_by BIGINT NOT NULL REFERENCES users(id),
  performed_at TIMESTAMPTZ NOT NULL DEFAULT now(), signed_at TIMESTAMPTZ NOT NULL DEFAULT now(), digital_signature TEXT NOT NULL, notes TEXT NOT NULL DEFAULT ''
);
CREATE OR REPLACE FUNCTION forbid_performed_treatment_mutation() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'Un procedimiento realizado es inmutable; registre una adenda'; END; $$ LANGUAGE plpgsql;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='performed_treatment_immutable') THEN
    EXECUTE 'CREATE TRIGGER performed_treatment_immutable BEFORE UPDATE OR DELETE ON treatment_performances FOR EACH ROW EXECUTE FUNCTION forbid_performed_treatment_mutation()';
  END IF;
END $$;
CREATE TABLE IF NOT EXISTS clinical_files (
  id BIGSERIAL PRIMARY KEY, patient_id BIGINT NOT NULL REFERENCES patients(id), uploaded_by BIGINT NOT NULL REFERENCES users(id),
  original_name TEXT NOT NULL, content_type TEXT NOT NULL, encrypted_path TEXT NOT NULL, sha256 TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS integration_events (
  id BIGSERIAL PRIMARY KEY, integration TEXT NOT NULL, event_type TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pendiente',
  payload JSONB NOT NULL DEFAULT '{}'::jsonb, response TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), processed_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS telemetry_events (
  id BIGSERIAL PRIMARY KEY, event_name TEXT NOT NULL, duration_ms INTEGER NOT NULL DEFAULT 0, status_code INTEGER NOT NULL DEFAULT 200,
  user_id BIGINT REFERENCES users(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_telemetry_created ON telemetry_events(created_at DESC);
