package controllers

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type patientRequestInput struct {
	Date   string `json:"date"`
	Time   string `json:"time"`
	Reason string `json:"reason"`
}

// PatientAppointmentRequests returns only the authenticated patient's own requests,
// or all requests to authorized reception staff.
func PatientAppointmentRequests(c *fiber.Ctx) error {
	role, _ := c.Locals("role").(string)
	if role == "paciente" {
		rows, err := repo.Query(`SELECT r.id, to_char(r.preferred_at AT TIME ZONE 'America/Guayaquil','YYYY-MM-DD'), to_char(r.preferred_at AT TIME ZONE 'America/Guayaquil','HH24:MI'), r.reason, coalesce(r.reason_enc,''::bytea), r.status, r.reception_note, r.created_at
      FROM patient_appointment_requests r WHERE r.user_id=$1 ORDER BY r.created_at DESC`, userID(c))
		if err != nil {
			return fail(c, 500, "No se pudieron consultar tus solicitudes")
		}
		defer rows.Close()
		out := []fiber.Map{}
		for rows.Next() {
			var id int64
			var date, clock, reason, status, note string
			var reasonEnc []byte
			var created time.Time
			if err := rows.Scan(&id, &date, &clock, &reason, &reasonEnc, &status, &note, &created); err != nil {
				return fail(c, 500, "Error leyendo solicitudes")
			}
			reason, err = decryptOrLegacy(reasonEnc, reason)
			if err != nil {
				return fail(c, 500, "No se pudo descifrar el motivo de la solicitud")
			}
			out = append(out, fiber.Map{"id": id, "date": date, "time": clock, "reason": reason, "status": status, "reception_note": note, "created_at": created})
		}
		if rows.Err() != nil {
			return fail(c, 500, "Error leyendo solicitudes")
		}
		return c.JSON(out)
	}
	rows, err := repo.Query(`SELECT r.id,r.user_id,u.full_name,u.email,to_char(r.preferred_at AT TIME ZONE 'America/Guayaquil','YYYY-MM-DD'),to_char(r.preferred_at AT TIME ZONE 'America/Guayaquil','HH24:MI'),r.reason,coalesce(r.reason_enc,''::bytea),r.status,r.reception_note,r.created_at
    FROM patient_appointment_requests r JOIN users u ON u.id=r.user_id ORDER BY CASE r.status WHEN 'pendiente' THEN 0 ELSE 1 END, r.preferred_at ASC LIMIT 500`)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar las solicitudes")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id, uid int64
		var name, email, date, clock, reason, status, note string
		var reasonEnc []byte
		var created time.Time
		if err := rows.Scan(&id, &uid, &name, &email, &date, &clock, &reason, &reasonEnc, &status, &note, &created); err != nil {
			return fail(c, 500, "Error leyendo solicitudes")
		}
		reason, err = decryptOrLegacy(reasonEnc, reason)
		if err != nil {
			return fail(c, 500, "No se pudo descifrar el motivo de la solicitud")
		}
		out = append(out, fiber.Map{"id": id, "user_id": uid, "patient_name": name, "email": email, "date": date, "time": clock, "reason": reason, "status": status, "reception_note": note, "created_at": created})
	}
	if rows.Err() != nil {
		return fail(c, 500, "Error leyendo solicitudes")
	}
	return c.JSON(out)
}

func CreatePatientAppointmentRequest(c *fiber.Ctx) error {
	var in patientRequestInput
	if err := c.BodyParser(&in); err != nil {
		return fail(c, 400, "Datos de solicitud inválidos")
	}
	in.Date = strings.TrimSpace(in.Date)
	in.Time = strings.TrimSpace(in.Time)
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) < 2 || len(in.Reason) > 300 {
		return fail(c, 400, "El motivo debe tener entre 2 y 300 caracteres")
	}
	preferred, err := time.ParseInLocation("2006-01-02 15:04", in.Date+" "+in.Time, time.FixedZone("Ecuador", -5*60*60))
	if err != nil || !preferred.After(time.Now()) {
		return fail(c, 400, "Selecciona una fecha y hora futuras válidas")
	}
	reasonEnc, err := dataCipher.Encrypt([]byte(in.Reason))
	if err != nil {
		return fail(c, 500, "No se pudo proteger el motivo de la solicitud")
	}
	var id int64
	var created time.Time
	err = repo.QueryRow(`INSERT INTO patient_appointment_requests(user_id,preferred_at,reason,reason_enc) VALUES($1,$2,'',$3) RETURNING id,created_at`, userID(c), preferred, reasonEnc).Scan(&id, &created)
	if err != nil {
		return fail(c, 500, "No se pudo guardar la solicitud en la base de datos")
	}
	audit(c, "crear", "patient_appointment_request", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id, "date": in.Date, "time": in.Time, "reason": in.Reason, "status": "pendiente", "created_at": created})
}

func ReviewPatientAppointmentRequest(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id < 1 {
		return fail(c, 400, "Solicitud inválida")
	}
	var in struct {
		Status string `json:"status"`
		Note   string `json:"reception_note"`
	}
	if err = c.BodyParser(&in); err != nil {
		return fail(c, 400, "Datos inválidos")
	}
	if in.Status != "confirmada" && in.Status != "rechazada" {
		return fail(c, 400, "El estado debe ser confirmada o rechazada")
	}
	in.Note = strings.TrimSpace(in.Note)
	if len(in.Note) > 500 {
		return fail(c, 400, "La nota no puede superar 500 caracteres")
	}
	res, err := repo.Exec(`UPDATE patient_appointment_requests SET status=$1,reception_note=$2,reviewed_by=$3,reviewed_at=now(),updated_at=now() WHERE id=$4 AND status='pendiente'`, in.Status, in.Note, userID(c), id)
	if err != nil {
		return fail(c, 500, "No se pudo actualizar la solicitud")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fail(c, 404, "No se encontró una solicitud pendiente")
	}
	audit(c, "actualizar", "patient_appointment_request", c.Params("id"))
	return c.JSON(fiber.Map{"message": "Solicitud actualizada", "status": in.Status})
}
