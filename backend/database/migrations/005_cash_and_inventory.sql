ALTER TABLE inventory_items ADD COLUMN IF NOT EXISTS unit_cost NUMERIC(12,4) NOT NULL DEFAULT 0 CHECK(unit_cost>=0);
ALTER TABLE payments ADD COLUMN IF NOT EXISTS cash_session_id BIGINT;
ALTER TABLE treatment_performances ADD CONSTRAINT uq_performance_appointment UNIQUE(appointment_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_settlement_professional_period ON professional_settlements(professional_id,period_start,period_end);
CREATE UNIQUE INDEX IF NOT EXISTS uq_settlement_treatment ON professional_settlement_items(settlement_id,treatment_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_settlement_treatment_global ON professional_settlement_items(treatment_id);
CREATE TABLE IF NOT EXISTS cash_sessions (
  id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL REFERENCES users(id), opening_float NUMERIC(12,2) NOT NULL CHECK(opening_float>=0),
  opened_at TIMESTAMPTZ NOT NULL DEFAULT now(), closed_at TIMESTAMPTZ, physical_count NUMERIC(12,2), expected_cash NUMERIC(12,2), variance NUMERIC(12,2),
  status TEXT NOT NULL DEFAULT 'abierta' CHECK(status IN ('abierta','cerrada'))
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_one_open_cash_session_per_user ON cash_sessions(user_id) WHERE status='abierta';
ALTER TABLE payments ADD CONSTRAINT fk_payments_cash_session FOREIGN KEY(cash_session_id) REFERENCES cash_sessions(id);
CREATE OR REPLACE FUNCTION forbid_inventory_movement_mutation() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'Un movimiento de inventario es inmutable'; END; $$ LANGUAGE plpgsql;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='inventory_movement_immutable') THEN
    EXECUTE 'CREATE TRIGGER inventory_movement_immutable BEFORE UPDATE OR DELETE ON inventory_movements FOR EACH ROW EXECUTE FUNCTION forbid_inventory_movement_mutation()';
  END IF;
END $$;
CREATE OR REPLACE FUNCTION forbid_treatment_approval_mutation() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'Una aprobación de tratamiento es inmutable'; END; $$ LANGUAGE plpgsql;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname='treatment_approval_immutable') THEN
    EXECUTE 'CREATE TRIGGER treatment_approval_immutable BEFORE UPDATE OR DELETE ON treatment_approvals FOR EACH ROW EXECUTE FUNCTION forbid_treatment_approval_mutation()';
  END IF;
END $$;
