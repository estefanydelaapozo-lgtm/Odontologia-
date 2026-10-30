package controllers

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"sigo-dental/backend/security"
)

func ListConsentTemplates(c *fiber.Ctx) error {
	rows, err := repo.Query(`SELECT id,code,procedure_name,version,body,active FROM consent_templates ORDER BY procedure_name,version DESC`)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar formatos")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id int64
		var code, name, version, body string
		var active bool
		if rows.Scan(&id, &code, &name, &version, &body, &active) == nil {
			out = append(out, fiber.Map{"id": id, "code": code, "procedure_name": name, "version": version, "body": body, "active": active})
		}
	}
	return c.JSON(out)
}
func CreateConsentTemplate(c *fiber.Ctx) error {
	var x struct {
		Code          string `json:"code"`
		ProcedureName string `json:"procedure_name"`
		Version       string `json:"version"`
		Body          string `json:"body"`
	}
	if c.BodyParser(&x) != nil || x.Code == "" || x.ProcedureName == "" || x.Version == "" || x.Body == "" {
		return fail(c, 400, "Código, procedimiento, versión y texto son obligatorios")
	}
	var id int64
	err := repo.QueryRow(`INSERT INTO consent_templates(code,procedure_name,version,body) VALUES($1,$2,$3,$4) RETURNING id`, x.Code, x.ProcedureName, x.Version, x.Body).Scan(&id)
	if err != nil {
		return fail(c, 409, "Ese formato ya existe o no pudo guardarse")
	}
	audit(c, "crear", "formato_consentimiento", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id})
}

func CreatePatientConsent(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	var x struct {
		TemplateID int64 `json:"template_id"`
	}
	if c.BodyParser(&x) != nil || x.TemplateID == 0 {
		return fail(c, 400, "Selecciona un formato de consentimiento")
	}
	var procedure, version, body string
	if err = repo.QueryRow(`SELECT procedure_name,version,body FROM consent_templates WHERE id=$1 AND active=TRUE`, x.TemplateID).Scan(&procedure, &version, &body); err != nil {
		return fail(c, 404, "Formato no encontrado")
	}
	snapshot, err := json.Marshal(fiber.Map{"procedure": procedure, "version": version, "text": body, "created_at": time.Now().UTC()})
	if err != nil {
		return fail(c, 500, "No se pudo crear el consentimiento")
	}
	encrypted, err := dataCipher.Encrypt(snapshot)
	if err != nil {
		return fail(c, 500, "No se pudo proteger el consentimiento")
	}
	var id int64
	err = repo.QueryRow(`INSERT INTO patient_consents(patient_id,template_id,template_version,snapshot,snapshot_enc,created_by) VALUES($1,$2,$3,'',$4,$5) RETURNING id`, patientID, x.TemplateID, version, encrypted, userID(c)).Scan(&id)
	if err != nil {
		return fail(c, 400, "No se pudo generar el consentimiento")
	}
	audit(c, "crear", "consentimiento", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id, "procedure": procedure, "status": "pendiente"})
}

func ListPatientConsents(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	rows, err := repo.Query(`SELECT id,template_version,snapshot_enc,status,coalesce(signature_method,''),signed_at,created_at FROM patient_consents WHERE patient_id=$1 ORDER BY created_at DESC`, patientID)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar consentimientos")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id int64
		var version, status, method string
		var encrypted []byte
		var signed sql.NullTime
		var created time.Time
		if rows.Scan(&id, &version, &encrypted, &status, &method, &signed, &created) != nil {
			continue
		}
		plain, e := dataCipher.Decrypt(encrypted)
		if e != nil {
			return fail(c, 500, "No se pudo descifrar un consentimiento")
		}
		var snapshot any
		if json.Unmarshal(plain, &snapshot) != nil {
			snapshot = string(plain)
		}
		out = append(out, fiber.Map{"id": id, "template_version": version, "snapshot": snapshot, "status": status, "signature_method": method, "signed_at": signed, "created_at": created})
	}
	audit(c, "consultar", "consentimiento", strconv.FormatInt(patientID, 10))
	return c.JSON(out)
}

func SendConsentOTP(c *fiber.Ctx) error {
	id, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de consentimiento inválido")
	}
	var patientID int64
	var status string
	if err = repo.QueryRow(`SELECT patient_id,status FROM patient_consents WHERE id=$1`, id).Scan(&patientID, &status); err != nil || status != "pendiente" {
		return fail(c, 404, "Consentimiento pendiente no encontrado")
	}
	var email, phone string
	var canContact bool
	if err = repo.QueryRow(`SELECT coalesce(email,''),coalesce(phone,''),consent_for_contact FROM patients WHERE id=$1`, patientID).Scan(&email, &phone, &canContact); err != nil {
		return fail(c, 404, "Paciente no encontrado")
	}
	if !canContact {
		return fail(c, 409, "El paciente no autorizó mensajes electrónicos; usa firma presencial")
	}
	channel := strings.ToLower(env("CONSENT_OTP_CHANNEL", "email"))
	destination := email
	if channel == "sms" || channel == "whatsapp" {
		destination = phone
	} else {
		channel = "email"
	}
	if destination == "" {
		return fail(c, 400, "El paciente no tiene un destino válido para este canal")
	}
	code, err := randomOTP()
	if err != nil {
		return fail(c, 500, "No se pudo generar el código")
	}
	hash := security.Sign(jwtSecret, []byte(strconv.FormatInt(id, 10)+"|"+code))
	payload, err := json.Marshal(fiber.Map{"subject": "Código de consentimiento informado", "message": "Tu código para firmar el consentimiento es " + code + ". Vence en 10 minutos.", "otp": code})
	if err != nil {
		return fail(c, 500, "No se pudo preparar el envío")
	}
	encrypted, err := dataCipher.Encrypt(payload)
	if err != nil {
		return fail(c, 500, "No se pudo proteger el envío")
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo encolar el envío")
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE consent_otp_attempts SET consumed_at=now() WHERE consent_id=$1 AND consumed_at IS NULL`, id); err != nil {
		return fail(c, 500, "No se pudo renovar el código")
	}
	if _, err = tx.Exec(`INSERT INTO consent_otp_attempts(consent_id,otp_hash,expires_at) VALUES($1,$2,now()+interval '10 minutes')`, id, hash); err != nil {
		return fail(c, 500, "No se pudo guardar el código")
	}
	if _, err = tx.Exec(`INSERT INTO notification_outbox(patient_id,channel,destination,template,payload,payload_enc,scheduled_at,idempotency_key) VALUES($1,$2,$3,'consent_otp','{}'::jsonb,$4,now(),$5)`, patientID, channel, destination, encrypted, "consent:"+strconv.FormatInt(id, 10)+":"+strconv.FormatInt(time.Now().UnixNano(), 10)); err != nil {
		return fail(c, 500, "No se pudo encolar el mensaje")
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar el envío")
	}
	audit(c, "solicitar_otp", "consentimiento", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"queued": true, "channel": channel, "destination_masked": maskDestination(destination)})
}

func SignPatientConsent(c *fiber.Ctx) error {
	id, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID inválido")
	}
	var x struct {
		Method    string `json:"method"`
		Code      string `json:"code"`
		Signature string `json:"signature"`
	}
	if c.BodyParser(&x) != nil {
		return fail(c, 400, "Firma inválida")
	}
	if x.Method != "otp" && x.Method != "tableta" {
		return fail(c, 400, "Método debe ser otp o tableta")
	}
	var patientID int64
	var status string
	if err = repo.QueryRow(`SELECT patient_id,status FROM patient_consents WHERE id=$1`, id).Scan(&patientID, &status); err != nil || status != "pendiente" {
		return fail(c, 404, "Consentimiento pendiente no encontrado")
	}
	if x.Method == "otp" {
		if x.Code == "" {
			return fail(c, 400, "Código OTP obligatorio")
		}
		var attemptID int64
		var hash string
		var attempts int
		if err = repo.QueryRow(`SELECT id,otp_hash,attempts FROM consent_otp_attempts WHERE consent_id=$1 AND consumed_at IS NULL AND expires_at>now() ORDER BY created_at DESC LIMIT 1`, id).Scan(&attemptID, &hash, &attempts); err != nil || attempts >= 5 {
			return fail(c, 401, "Código no disponible o vencido")
		}
		candidate := security.Sign(jwtSecret, []byte(strconv.FormatInt(id, 10)+"|"+x.Code))
		if subtle.ConstantTimeCompare([]byte(hash), []byte(candidate)) != 1 {
			_, _ = repo.Exec(`UPDATE consent_otp_attempts SET attempts=attempts+1 WHERE id=$1`, attemptID)
			return fail(c, 401, "Código incorrecto")
		}
		_, _ = repo.Exec(`UPDATE consent_otp_attempts SET consumed_at=now() WHERE id=$1`, attemptID)
	} else if len(strings.TrimSpace(x.Signature)) < 20 {
		return fail(c, 400, "Captura la firma del paciente en la pantalla")
	}
	signatureEnc, err := dataCipher.Encrypt([]byte(x.Signature))
	if err != nil {
		return fail(c, 500, "No se pudo proteger la firma")
	}
	res, err := repo.Exec(`UPDATE patient_consents SET status='firmado',signature_method=$1,signature_data='',signature_data_enc=$2,signed_at=now() WHERE id=$3 AND status='pendiente'`, x.Method, signatureEnc, id)
	if err != nil {
		return fail(c, 500, "No se pudo registrar la firma")
	}
	count, _ := res.RowsAffected()
	if count == 0 {
		return fail(c, 409, "El consentimiento ya cambió de estado")
	}
	audit(c, "firmar", "consentimiento", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"id": id, "status": "firmado", "signed_at": time.Now().UTC()})
}

func maskDestination(value string) string {
	if strings.Contains(value, "@") {
		parts := strings.SplitN(value, "@", 2)
		if len(parts[0]) < 3 {
			return "***@" + parts[1]
		}
		return parts[0][:1] + "***@" + parts[1]
	}
	if len(value) < 4 {
		return "***"
	}
	return strings.Repeat("*", len(value)-3) + value[len(value)-3:]
}
