package controllers

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

func roleOf(c *fiber.Ctx) string { r, _ := c.Locals("role").(string); return r }

// queryRows ejecuta una consulta parametrizada y devuelve filas como mapas (+ el orden de columnas).
func queryRows(query string, args ...any) ([]map[string]any, []string, error) {
	rows, err := repo.Query(query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		m := make(map[string]any, len(cols))
		for i, col := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[col] = string(b)
			} else {
				m[col] = vals[i]
			}
		}
		out = append(out, m)
	}
	return out, cols, rows.Err()
}

func dateRange(c *fiber.Ctx) (time.Time, time.Time) {
	loc := clinicLocation()
	now := time.Now().In(loc)
	hasta := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	desde := hasta.AddDate(0, 0, -31)
	if t, err := time.ParseInLocation("2006-01-02", c.Query("desde"), loc); err == nil {
		desde = t
	}
	if t, err := time.ParseInLocation("2006-01-02", c.Query("hasta"), loc); err == nil {
		hasta = t.AddDate(0, 0, 1)
	}
	return desde, hasta
}

// ---------------------------------------------------------------- Auditoría (módulo 17)

// ListAuditLog permite a administración y auditoría consultar el log inmutable con filtros.
func ListAuditLog(c *fiber.Ctx) error {
	desde, hasta := dateRange(c)
	q := `SELECT al.id, al.created_at, coalesce(u.full_name,'(sin usuario)') AS usuario, coalesce(u.email,'') AS correo,
	      al.action AS accion, al.entity AS entidad, al.entity_id AS referencia, al.ip_address AS ip
	      FROM audit_log al LEFT JOIN users u ON u.id = al.user_id WHERE al.created_at >= $1 AND al.created_at < $2`
	args := []any{desde, hasta}
	if v := strings.TrimSpace(c.Query("usuario")); v != "" {
		args = append(args, "%"+v+"%")
		q += fmt.Sprintf(` AND (u.full_name ILIKE $%d OR u.email ILIKE $%d)`, len(args), len(args))
	}
	if v := strings.TrimSpace(c.Query("accion")); v != "" {
		args = append(args, "%"+v+"%")
		q += fmt.Sprintf(` AND al.action ILIKE $%d`, len(args))
	}
	if v := strings.TrimSpace(c.Query("entidad")); v != "" {
		args = append(args, "%"+v+"%")
		q += fmt.Sprintf(` AND al.entity ILIKE $%d`, len(args))
	}
	limit := 200
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 && n <= 1000 {
		limit = n
	}
	args = append(args, limit)
	q += fmt.Sprintf(` ORDER BY al.id DESC LIMIT $%d`, len(args))
	rows, _, err := queryRows(q, args...)
	if err != nil {
		return fail(c, 500, "No se pudo consultar la auditoría")
	}
	audit(c, "consultar", "auditoria", "listado")
	return c.JSON(rows)
}

// ---------------------------------------------------------------- Expediente unificado (módulo 4)

func decryptOrLegacy(enc []byte, legacy string) (string, error) {
	if len(enc) == 0 {
		return legacy, nil
	}
	plain, err := dataCipher.Decrypt(enc)
	return string(plain), err
}

// PatientRecord devuelve el historial completo del paciente en una sola respuesta. Cada lectura queda auditada;
// con ?export=1 se entrega como descarga JSON y se audita como exportación.
func PatientRecord(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fail(c, 400, "ID inválido")
	}
	var p struct {
		ID                                                       int64
		Doc, First, Last, Phone, Email, EmContact, EmPhone, Guard string
		Insurance, Policy, LA, LL, LH                            string
		Birth                                                    sql.NullTime
		Consent                                                  bool
		AE, LE, HE                                               []byte
	}
	err = repo.QueryRow(`SELECT id,coalesce(document_id,''),first_name,last_name,birth_date,coalesce(phone,''),coalesce(email,''),coalesce(emergency_contact,''),coalesce(emergency_phone,''),coalesce(guardian_name,''),coalesce(insurance,''),coalesce(insurance_policy,''),
		medical_alerts,allergies,medical_history,coalesce(medical_alerts_enc,''::bytea),coalesce(allergies_enc,''::bytea),coalesce(medical_history_enc,''::bytea),consent_for_contact FROM patients WHERE id=$1`, id).
		Scan(&p.ID, &p.Doc, &p.First, &p.Last, &p.Birth, &p.Phone, &p.Email, &p.EmContact, &p.EmPhone, &p.Guard, &p.Insurance, &p.Policy, &p.LA, &p.LL, &p.LH, &p.AE, &p.LE, &p.HE, &p.Consent)
	if err == sql.ErrNoRows {
		return fail(c, 404, "Paciente no encontrado")
	}
	if err != nil {
		return fail(c, 500, "No se pudo consultar el paciente")
	}
	alerts, e1 := decryptOrLegacy(p.AE, p.LA)
	allergies, e2 := decryptOrLegacy(p.LE, p.LL)
	history, e3 := decryptOrLegacy(p.HE, p.LH)
	if e1 != nil || e2 != nil || e3 != nil {
		return fail(c, 500, "No se pudo descifrar información clínica")
	}
	badges := []string{}
	if t := strings.TrimSpace(alerts); t != "" {
		badges = append(badges, t)
	}
	if t := strings.TrimSpace(allergies); t != "" && !strings.EqualFold(t, "ninguna") && !strings.EqualFold(t, "ninguno") {
		badges = append(badges, "Alergias: "+t)
	}
	appointments, _, err := queryRows(`SELECT a.id,a.starts_at,a.ends_at,a.status,coalesce(s.name,'') AS servicio,pr.full_name AS profesional,c.name AS sillon
		FROM appointments a JOIN professionals pr ON pr.id=a.professional_id JOIN chairs c ON c.id=a.chair_id LEFT JOIN services s ON s.id=a.service_id
		WHERE a.patient_id=$1 ORDER BY a.starts_at DESC LIMIT 100`, id)
	if err != nil {
		return fail(c, 500, "No se pudo consultar las citas")
	}
	plans, _, err := queryRows(`SELECT tp.id,tp.title,tp.status,tp.created_at,
		coalesce((SELECT json_agg(json_build_object('id',i.id,'servicio',s.name,'fase',i.phase_name,'sesion',i.session_number,'estado',i.status,'honorario',i.professional_fee,'laboratorio',i.laboratory_cost,'descuento',i.discount) ORDER BY i.phase_name,i.session_number,i.id)
		FROM treatment_plan_items i JOIN services s ON s.id=i.service_id WHERE i.plan_id=tp.id),'[]'::json)::text AS items
		FROM treatment_plans tp WHERE tp.patient_id=$1 ORDER BY tp.id DESC`, id)
	if err != nil {
		return fail(c, 500, "No se pudo consultar los planes")
	}
	treatments, _, err := queryRows(`SELECT t.id,t.performed_at,s.name AS servicio,pr.full_name AS profesional FROM treatment_performances t
		JOIN services s ON s.id=t.service_id JOIN professionals pr ON pr.id=t.professional_id WHERE t.patient_id=$1 ORDER BY t.performed_at DESC`, id)
	if err != nil {
		return fail(c, 500, "No se pudo consultar los tratamientos")
	}
	invoices, _, err := queryRows(`SELECT i.id,i.created_at,i.total,i.status,coalesce((SELECT sum(amount) FROM payments WHERE invoice_id=i.id),0) AS pagado FROM invoices i WHERE i.patient_id=$1 ORDER BY i.id DESC`, id)
	if err != nil {
		return fail(c, 500, "No se pudo consultar los cobros")
	}
	var notes, consents, files int
	_ = repo.QueryRow(`SELECT count(*) FROM evolution_notes WHERE patient_id=$1`, id).Scan(&notes)
	_ = repo.QueryRow(`SELECT count(*) FROM patient_consents WHERE patient_id=$1`, id).Scan(&consents)
	_ = repo.QueryRow(`SELECT count(*) FROM clinical_files WHERE patient_id=$1`, id).Scan(&files)
	var birth any
	if p.Birth.Valid {
		birth = p.Birth.Time.Format("2006-01-02")
	}
	out := fiber.Map{
		"patient": fiber.Map{"id": p.ID, "document_id": p.Doc, "first_name": p.First, "last_name": p.Last, "birth_date": birth, "phone": p.Phone, "email": p.Email,
			"emergency_contact": p.EmContact, "emergency_phone": p.EmPhone, "guardian_name": p.Guard, "insurance": p.Insurance, "insurance_policy": p.Policy,
			"medical_alerts": alerts, "allergies": allergies, "medical_history": history, "consent_for_contact": p.Consent},
		"alerts": badges, "appointments": appointments, "treatment_plans": plans, "treatments": treatments, "invoices": invoices,
		"counts": fiber.Map{"clinical_notes": notes, "consents": consents, "files": files}, "generated_at": time.Now().UTC().Format(time.RFC3339),
	}
	if c.Query("export") == "1" {
		audit(c, "exportar", "expediente", strconv.FormatInt(id, 10))
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="expediente-%d.json"`, id))
	} else {
		audit(c, "consultar", "expediente", strconv.FormatInt(id, 10))
	}
	return c.JSON(out)
}

// ---------------------------------------------------------------- Reportes (módulo 15)

type reportDef struct {
	title  string
	query  string
	ranged bool
	roles  []string
}

var reportDefs = map[string]reportDef{
	"patients": {"Pacientes registrados", `SELECT id, coalesce(document_id,'') AS documento, first_name AS nombres, last_name AS apellidos, birth_date AS nacimiento,
		coalesce(phone,'') AS telefono, coalesce(insurance,'') AS seguro, created_at AS registrado FROM patients WHERE created_at >= $1 AND created_at < $2 ORDER BY created_at DESC`, true,
		[]string{"admin", "recepcionista", "auditor"}},
	"appointments": {"Citas", `SELECT a.id, a.starts_at AS inicio, a.ends_at AS fin, p.first_name || ' ' || p.last_name AS paciente, pr.full_name AS profesional,
		ch.name AS sillon, coalesce(s.name,'') AS servicio, a.status AS estado FROM appointments a JOIN patients p ON p.id=a.patient_id
		JOIN professionals pr ON pr.id=a.professional_id JOIN chairs ch ON ch.id=a.chair_id LEFT JOIN services s ON s.id=a.service_id
		WHERE a.starts_at >= $1 AND a.starts_at < $2 ORDER BY a.starts_at`, true, []string{"admin", "recepcionista", "auditor"}},
	"income": {"Ingresos por pagos", `SELECT pay.id, pay.created_at AS fecha, pay.invoice_id AS cobro, p.first_name || ' ' || p.last_name AS paciente,
		pay.method AS metodo, pay.amount AS monto, pay.reference AS referencia FROM payments pay JOIN invoices i ON i.id=pay.invoice_id
		JOIN patients p ON p.id=i.patient_id WHERE pay.created_at >= $1 AND pay.created_at < $2 AND i.status <> 'anulada' ORDER BY pay.created_at`, true, []string{"admin", "auditor"}},
	"treatments": {"Tratamientos realizados", `SELECT t.id, t.performed_at AS fecha, p.first_name || ' ' || p.last_name AS paciente, s.name AS servicio,
		pr.full_name AS profesional, s.price AS tarifa_base FROM treatment_performances t JOIN patients p ON p.id=t.patient_id JOIN services s ON s.id=t.service_id
		JOIN professionals pr ON pr.id=t.professional_id WHERE t.performed_at >= $1 AND t.performed_at < $2 ORDER BY t.performed_at DESC`, true, []string{"admin", "auditor"}},
	"inventory": {"Inventario y existencias", `SELECT i.sku, i.name AS insumo, i.unit AS unidad, coalesce(sum(b.quantity),0) AS existencias, i.minimum_stock AS minimo,
		CASE WHEN coalesce(sum(b.quantity),0) <= i.minimum_stock THEN 'Stock bajo' ELSE 'Disponible' END AS estado
		FROM inventory_items i LEFT JOIN inventory_balances b ON b.item_id=i.id WHERE i.active GROUP BY i.id ORDER BY i.name`, false, []string{"admin", "auditor", "asistente"}},
}

func csvSafe(v any) string {
	s := ""
	switch x := v.(type) {
	case nil:
		s = ""
	case time.Time:
		s = x.Format(time.RFC3339)
	default:
		s = fmt.Sprint(x)
	}
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s // evita inyección de fórmulas al abrir el CSV en Excel
	}
	return s
}

// Report entrega un reporte en JSON o, con ?format=csv, como archivo CSV (la exportación queda auditada).
func Report(c *fiber.Ctx) error {
	kind := c.Params("kind")
	def, ok := reportDefs[kind]
	if !ok {
		return fail(c, 404, "Reporte inexistente")
	}
	allowed := false
	for _, r := range def.roles {
		if r == roleOf(c) {
			allowed = true
		}
	}
	if !allowed {
		return fail(c, 403, "No tienes permiso para este reporte")
	}
	args := []any{}
	if def.ranged {
		d, h := dateRange(c)
		args = append(args, d, h)
	}
	rows, cols, err := queryRows(def.query, args...)
	if err != nil {
		return fail(c, 500, "No se pudo generar el reporte")
	}
	if c.Query("format") == "csv" {
		audit(c, "exportar", "reporte:"+kind, "csv")
		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="reporte-%s-%s.csv"`, kind, time.Now().Format("20060102")))
		var sb strings.Builder
		sb.WriteString("\ufeff")
		w := csv.NewWriter(&sb)
		_ = w.Write(cols)
		for _, r := range rows {
			rec := make([]string, len(cols))
			for i, col := range cols {
				rec[i] = csvSafe(r[col])
			}
			_ = w.Write(rec)
		}
		w.Flush()
		return c.SendString(sb.String())
	}
	audit(c, "consultar", "reporte:"+kind, "json")
	return c.JSON(fiber.Map{"kind": kind, "title": def.title, "columns": cols, "rows": rows})
}

// ---------------------------------------------------------------- Usuarios y roles (módulo 16)

var validRoles = map[string]bool{"admin": true, "recepcionista": true, "odontologo": true, "asistente": true, "auditor": true}

// UpdateUser cambia nombre, rol o estado activo (un administrador no puede quitarse a sí mismo el acceso).
func UpdateUser(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fail(c, 400, "ID inválido")
	}
	var in struct {
		FullName string `json:"full_name"`
		Role     string `json:"role"`
		Active   *bool  `json:"active"`
	}
	if err := c.BodyParser(&in); err != nil || strings.TrimSpace(in.FullName) == "" || !validRoles[in.Role] || in.Active == nil {
		return fail(c, 400, "Nombre, rol válido y estado son obligatorios")
	}
	if id == userID(c) && (!*in.Active || in.Role != "admin") {
		return fail(c, 400, "No puedes desactivar tu cuenta ni quitarte el rol de administrador")
	}
	res, err := repo.Exec(`UPDATE users SET full_name=$1, role=$2, active=$3 WHERE id=$4`, strings.TrimSpace(in.FullName), in.Role, *in.Active, id)
	if err != nil {
		return fail(c, 500, "No se pudo actualizar el usuario")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(c, 404, "Usuario no encontrado")
	}
	audit(c, "actualizar", "usuarios", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"ok": true})
}

// ResetUserPassword fija una nueva contraseña (bcrypt) para un usuario.
func ResetUserPassword(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fail(c, 400, "ID inválido")
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&in); err != nil || len(in.Password) < 8 {
		return fail(c, 400, "La contraseña debe tener al menos 8 caracteres")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return fail(c, 500, "No se pudo proteger la contraseña")
	}
	res, err := repo.Exec(`UPDATE users SET password_hash=$1 WHERE id=$2`, string(hash), id)
	if err != nil {
		return fail(c, 500, "No se pudo cambiar la contraseña")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(c, 404, "Usuario no encontrado")
	}
	audit(c, "restablecer_clave", "usuarios", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"ok": true})
}

// ResetUserMFA permite recuperar el acceso de un usuario que perdió su dispositivo autenticador.
func ResetUserMFA(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fail(c, 400, "ID inválido")
	}
	res, err := repo.Exec(`UPDATE users SET mfa_enabled=FALSE, mfa_secret_ciphertext=NULL, mfa_setup_secret_enc=NULL WHERE id=$1`, id)
	if err != nil {
		return fail(c, 500, "No se pudo restablecer el MFA")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(c, 404, "Usuario no encontrado")
	}
	audit(c, "restablecer_mfa", "usuarios", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"ok": true})
}

// ---------------------------------------------------------------- Honorarios, inventario y notificaciones

func ListFeeRules(c *fiber.Ctx) error {
	rows, _, err := queryRows(`SELECT r.id, pr.full_name AS profesional, coalesce(s.name,'Todos los servicios') AS servicio, r.rule_type AS tipo, r.rate AS tarifa, r.active AS activa, r.valid_from AS vigente_desde
		FROM professional_fee_rules r JOIN professionals pr ON pr.id=r.professional_id LEFT JOIN services s ON s.id=r.service_id ORDER BY pr.full_name, s.name NULLS FIRST, r.id DESC`)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar las reglas")
	}
	return c.JSON(rows)
}

func DeactivateFeeRule(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fail(c, 400, "ID inválido")
	}
	res, err := repo.Exec(`UPDATE professional_fee_rules SET active=FALSE WHERE id=$1`, id)
	if err != nil {
		return fail(c, 500, "No se pudo desactivar la regla")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(c, 404, "Regla no encontrada")
	}
	audit(c, "desactivar", "reglas_honorarios", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"ok": true})
}

func ListInventoryMovements(c *fiber.Ctx) error {
	rows, _, err := queryRows(`SELECT m.id, m.created_at AS fecha, i.name AS insumo, i.unit AS unidad, m.movement_type AS tipo, m.quantity AS cantidad,
		coalesce(ch.name,'Bodega') AS ubicacion, m.reference_type AS origen, m.reference_id AS referencia, u.full_name AS registrado_por
		FROM inventory_movements m JOIN inventory_items i ON i.id=m.item_id LEFT JOIN chairs ch ON ch.id=m.chair_id JOIN users u ON u.id=m.recorded_by ORDER BY m.id DESC LIMIT 300`)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar los movimientos")
	}
	return c.JSON(rows)
}

func ListNotifications(c *fiber.Ctx) error {
	rows, _, err := queryRows(`SELECT n.id, n.channel AS canal, n.template AS plantilla, n.destination, n.scheduled_at AS programada, n.sent_at AS enviada, n.status AS estado,
		n.provider_response AS respuesta, coalesce(p.first_name || ' ' || p.last_name,'') AS paciente FROM notification_outbox n LEFT JOIN patients p ON p.id=n.patient_id ORDER BY n.scheduled_at DESC LIMIT 200`)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar las notificaciones")
	}
	for _, r := range rows {
		if d, ok := r["destination"].(string); ok {
			r["destination"] = maskDestination(d) // el destino se muestra enmascarado
		}
	}
	return c.JSON(rows)
}

// ---------------------------------------------------------------- Agenda: reprogramar cita (RF-09)

// RescheduleAppointment cambia horario, profesional o sillón de una cita aún no iniciada, validando RN-03.
func RescheduleAppointment(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fail(c, 400, "ID inválido")
	}
	var in struct {
		StartsAt       time.Time `json:"starts_at"`
		EndsAt         time.Time `json:"ends_at"`
		ProfessionalID int64     `json:"professional_id"`
		ChairID        int64     `json:"chair_id"`
		Notes          string    `json:"notes"`
	}
	if err := c.BodyParser(&in); err != nil || in.StartsAt.IsZero() || !in.EndsAt.After(in.StartsAt) || in.ProfessionalID == 0 || in.ChairID == 0 {
		return fail(c, 400, "Fecha, hora, profesional y sillón son obligatorios; el fin debe ser posterior al inicio")
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo reprogramar")
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRow(`SELECT status FROM appointments WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return fail(c, 404, "Cita no encontrada")
	}
	if status != "programado" && status != "confirmado" {
		return fail(c, 409, "Solo se pueden reprogramar citas programadas o confirmadas")
	}
	var conflicts int
	if err := tx.QueryRow(`SELECT count(*) FROM appointments WHERE id<>$5 AND status NOT IN ('cancelado','no_asistio') AND starts_at < $2 AND ends_at > $1 AND (chair_id=$3 OR professional_id=$4)`,
		in.StartsAt, in.EndsAt, in.ChairID, in.ProfessionalID, id).Scan(&conflicts); err != nil {
		return fail(c, 500, "No se pudo validar la disponibilidad")
	}
	if conflicts > 0 {
		return fail(c, 409, "El sillón o el profesional ya tiene una cita en ese horario")
	}
	if _, err := tx.Exec(`UPDATE appointments SET starts_at=$1, ends_at=$2, professional_id=$3, chair_id=$4, notes=$5, status='programado', reminder_48h_sent_at=NULL, reminder_24h_sent_at=NULL WHERE id=$6`,
		in.StartsAt, in.EndsAt, in.ProfessionalID, in.ChairID, in.Notes, id); err != nil {
		return fail(c, 409, "No se pudo reprogramar: horario ocupado o datos inválidos")
	}
	if err := tx.Commit(); err != nil {
		return fail(c, 500, "No se pudo confirmar la reprogramación")
	}
	audit(c, "reprogramar", "citas", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"ok": true})
}
