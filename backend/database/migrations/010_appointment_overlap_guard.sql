-- RN-03 a nivel de base de datos: un sillón o un profesional no pueden tener dos citas activas
-- en el mismo horario, aunque dos peticiones lleguen a la vez (el control de la API sigue existiendo).
CREATE OR REPLACE FUNCTION guard_appointment_overlap() RETURNS trigger AS $$
BEGIN
  IF NEW.status IN ('cancelado','no_asistio') THEN
    RETURN NEW;
  END IF;
  PERFORM pg_advisory_xact_lock(hashtext('appointment_chair:' || NEW.chair_id::text));
  PERFORM pg_advisory_xact_lock(hashtext('appointment_professional:' || NEW.professional_id::text));
  IF EXISTS (
    SELECT 1 FROM appointments a
     WHERE a.id <> COALESCE(NEW.id, -1)
       AND a.status NOT IN ('cancelado','no_asistio')
       AND a.starts_at < NEW.ends_at AND a.ends_at > NEW.starts_at
       AND (a.chair_id = NEW.chair_id OR a.professional_id = NEW.professional_id)
  ) THEN
    RAISE EXCEPTION 'El sillón o el profesional ya tiene una cita en ese horario' USING ERRCODE = '23P01';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS appointments_overlap_guard ON appointments;
CREATE TRIGGER appointments_overlap_guard
  BEFORE INSERT OR UPDATE OF chair_id, professional_id, starts_at, ends_at, status ON appointments
  FOR EACH ROW EXECUTE FUNCTION guard_appointment_overlap();
