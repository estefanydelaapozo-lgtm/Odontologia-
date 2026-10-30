package controllers

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"sigo-dental/backend/domain"
	"sigo-dental/backend/security"
)

type ToothFinding struct {
	ToothCode string   `json:"tooth_code"`
	Condition string   `json:"condition"`
	Surfaces  []string `json:"surfaces"`
	Notes     string   `json:"notes"`
}
type PeriodontalPoint struct {
	ToothCode          string  `json:"tooth_code"`
	Site               string  `json:"site"`
	ProbingDepth       float64 `json:"probing_depth_mm"`
	GingivalMargin     float64 `json:"gingival_margin_mm"`
	ClinicalAttachment float64 `json:"clinical_attachment_level_mm"`
	Furcation          string  `json:"furcation"`
	Mobility           int     `json:"mobility"`
	Bleeding           bool    `json:"bleeding"`
}

func idFromParam(c *fiber.Ctx, name string) (int64, error) {
	return strconv.ParseInt(c.Params(name), 10, 64)
}

func GetOdontogram(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	notation := strings.ToUpper(c.Query("notation", "FDI"))
	dentition := strings.ToLower(c.Query("dentition", "permanente"))
	if notation != "FDI" && notation != "UNIVERSAL" {
		return fail(c, 400, "Usa FDI o Universal")
	}
	var chartID int64
	var payload []byte
	var updated time.Time
	err = repo.QueryRow(`SELECT id,coalesce(chart_data_enc,''::bytea),updated_at FROM dental_charts WHERE patient_id=$1 AND dentition=$2 AND notation=$3`, patientID, dentition, notation).Scan(&chartID, &payload, &updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(fiber.Map{"patient_id": patientID, "dentition": dentition, "notation": notation, "findings": []ToothFinding{}})
		}
		return fail(c, 500, "No se pudo leer el odontograma")
	}
	plain, err := dataCipher.Decrypt(payload)
	if err != nil {
		return fail(c, 500, "No se pudo descifrar el odontograma")
	}
	var findings []ToothFinding
	if json.Unmarshal(plain, &findings) != nil {
		return fail(c, 500, "El odontograma almacenado no es válido")
	}
	if notation == "UNIVERSAL" {
		for i := range findings {
			findings[i].ToothCode, err = domain.ToUniversal(findings[i].ToothCode)
			if err != nil {
				return fail(c, 500, "No se pudo convertir la notación dental")
			}
		}
	}
	audit(c, "consultar", "odontograma", strconv.FormatInt(patientID, 10))
	return c.JSON(fiber.Map{"id": chartID, "patient_id": patientID, "dentition": dentition, "notation": notation, "findings": findings, "updated_at": updated})
}

func SaveOdontogram(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	var input struct {
		Dentition string         `json:"dentition"`
		Notation  string         `json:"notation"`
		Findings  []ToothFinding `json:"findings"`
	}
	if c.BodyParser(&input) != nil {
		return fail(c, 400, "Odontograma inválido")
	}
	input.Dentition = strings.ToLower(input.Dentition)
	input.Notation = strings.ToUpper(input.Notation)
	if input.Dentition != "permanente" && input.Dentition != "temporal" && input.Dentition != "mixta" {
		return fail(c, 400, "Dentición no válida")
	}
	if input.Notation != "FDI" && input.Notation != "UNIVERSAL" {
		return fail(c, 400, "Notación no válida")
	}
	seen := map[string]bool{}
	for i := range input.Findings {
		f := &input.Findings[i]
		normalized, e := domain.NormalizeToFDI(f.ToothCode, input.Notation)
		if e != nil {
			return fail(c, 400, e.Error())
		}
		if seen[normalized] {
			return fail(c, 400, "No repitas piezas dentales")
		}
		seen[normalized] = true
		if !allowedDentalCondition(f.Condition) {
			return fail(c, 400, "Condición dental no válida")
		}
		if e = domain.ValidateSurfaces(f.Surfaces); e != nil {
			return fail(c, 400, e.Error())
		}
		f.ToothCode = normalized
		quadrant := normalized[0]
		if input.Dentition == "permanente" && quadrant >= '5' || input.Dentition == "temporal" && quadrant <= '4' {
			return fail(c, 400, "La pieza no coincide con la dentición elegida")
		}
	}
	payload, err := json.Marshal(input.Findings)
	if err != nil {
		return fail(c, 400, "No se pudo procesar el odontograma")
	}
	encrypted, err := dataCipher.Encrypt(payload)
	if err != nil {
		return fail(c, 500, "No se pudo proteger el odontograma")
	}
	var chartID int64
	err = repo.QueryRow(`INSERT INTO dental_charts(patient_id,dentition,notation,chart_data,chart_data_enc,created_by,updated_by) VALUES($1,$2,$3,'{}'::jsonb,$4,$5,$5) ON CONFLICT(patient_id,dentition,notation) DO UPDATE SET chart_data_enc=EXCLUDED.chart_data_enc,updated_by=EXCLUDED.updated_by,updated_at=now() RETURNING id`, patientID, input.Dentition, input.Notation, encrypted, userID(c)).Scan(&chartID)
	if err != nil {
		return fail(c, 400, "No se pudo guardar el odontograma")
	}
	audit(c, "actualizar", "odontograma", strconv.FormatInt(patientID, 10))
	return c.JSON(fiber.Map{"id": chartID, "findings": len(input.Findings), "saved": true})
}
func allowedDentalCondition(v string) bool {
	switch v {
	case "caries", "restauracion", "endodoncia_realizada", "endodoncia_requerida", "implante", "protesis_fija", "protesis_removible", "exodoncia", "sellante", "sano", "otro":
		return true
	}
	return false
}

func SavePeriodontogram(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	var input struct {
		Measurements []PeriodontalPoint `json:"measurements"`
	}
	if c.BodyParser(&input) != nil || len(input.Measurements) == 0 {
		return fail(c, 400, "Incluye los puntos medidos")
	}
	sites := map[string]bool{"mesiovestibular": true, "vestibular": true, "distovestibular": true, "mesiolingual": true, "lingual": true, "distolingual": true}
	counts := map[string]map[string]bool{}
	for i := range input.Measurements {
		m := &input.Measurements[i]
		tooth, e := domain.NormalizeToFDI(m.ToothCode, "FDI")
		if e != nil {
			return fail(c, 400, e.Error())
		}
		m.ToothCode = tooth
		if !sites[m.Site] || m.ProbingDepth < 0 || m.ProbingDepth > 20 || m.GingivalMargin < -10 || m.GingivalMargin > 10 || m.Mobility < 0 || m.Mobility > 3 || (m.Furcation != "" && m.Furcation != "ninguna" && m.Furcation != "I" && m.Furcation != "II" && m.Furcation != "III") {
			return fail(c, 400, "Punto periodontal fuera de rango")
		}
		if counts[tooth] == nil {
			counts[tooth] = map[string]bool{}
		}
		if counts[tooth][m.Site] {
			return fail(c, 400, "Punto periodontal repetido")
		}
		counts[tooth][m.Site] = true
		m.ClinicalAttachment = domain.ClinicalAttachmentLevel(m.ProbingDepth, m.GingivalMargin)
	}
	for _, six := range counts {
		if len(six) != 6 {
			return fail(c, 400, "Cada diente debe incluir sus seis puntos de sondaje")
		}
	}
	payload, err := json.Marshal(input.Measurements)
	if err != nil {
		return fail(c, 400, "Mediciones inválidas")
	}
	encrypted, err := dataCipher.Encrypt(payload)
	if err != nil {
		return fail(c, 500, "No se pudieron proteger las mediciones")
	}
	var id int64
	err = repo.QueryRow(`INSERT INTO patient_periodontograms(patient_id,measurements_enc,recorded_by) VALUES($1,$2,$3) RETURNING id`, patientID, encrypted, userID(c)).Scan(&id)
	if err != nil {
		return fail(c, 500, "No se pudo guardar el periodontograma")
	}
	audit(c, "crear", "periodontograma", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id, "teeth": len(counts), "points": len(input.Measurements)})
}
func GetPeriodontogram(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	var id int64
	var payload []byte
	var at time.Time
	err = repo.QueryRow(`SELECT id,measurements_enc,measured_at FROM patient_periodontograms WHERE patient_id=$1 ORDER BY measured_at DESC LIMIT 1`, patientID).Scan(&id, &payload, &at)
	if err != nil {
		return c.JSON(fiber.Map{"patient_id": patientID, "measurements": []PeriodontalPoint{}})
	}
	plain, err := dataCipher.Decrypt(payload)
	if err != nil {
		return fail(c, 500, "No se pudo descifrar el periodontograma")
	}
	var measurements []PeriodontalPoint
	if json.Unmarshal(plain, &measurements) != nil {
		return fail(c, 500, "Mediciones guardadas inválidas")
	}
	audit(c, "consultar", "periodontograma", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"id": id, "patient_id": patientID, "measured_at": at, "measurements": measurements})
}

type SOEPNote struct {
	ID            int64              `json:"id"`
	PatientID     int64              `json:"patient_id"`
	AppointmentID *int64             `json:"appointment_id"`
	Subjective    string             `json:"subjective"`
	Objective     string             `json:"objective"`
	Evaluation    string             `json:"evaluation"`
	Plan          string             `json:"plan"`
	AuthorID      int64              `json:"author_id"`
	SignedAt      *time.Time         `json:"signed_at"`
	Signature     string             `json:"signature"`
	CreatedAt     time.Time          `json:"created_at"`
	Addenda       []ClinicalAddendum `json:"addenda"`
}

type ClinicalAddendum struct {
	ID         int64     `json:"id"`
	Correction string    `json:"correction"`
	SignedBy   int64     `json:"signed_by"`
	Signature  string    `json:"signature"`
	CreatedAt  time.Time `json:"created_at"`
}

func clinicalNoteEditHours() (int, error) {
	hours, err := strconv.Atoi(env("CLINICAL_NOTE_EDIT_HOURS", "24"))
	if err != nil || hours < 1 || hours > 720 {
		return 0, fmt.Errorf("CLINICAL_NOTE_EDIT_HOURS debe ser un entero entre 1 y 720")
	}
	return hours, nil
}

func CreateClinicalNote(c *fiber.Ctx) error {
	var n SOEPNote
	if c.BodyParser(&n) != nil || n.PatientID == 0 || strings.TrimSpace(n.Subjective) == "" || strings.TrimSpace(n.Objective) == "" || strings.TrimSpace(n.Evaluation) == "" || strings.TrimSpace(n.Plan) == "" {
		return fail(c, 400, "Completa los cuatro apartados SOEP")
	}
	enc := make([][]byte, 4)
	vals := []string{n.Subjective, n.Objective, n.Evaluation, n.Plan}
	for i, v := range vals {
		b, e := dataCipher.Encrypt([]byte(v))
		if e != nil {
			return fail(c, 500, "No se pudo proteger la nota clínica")
		}
		enc[i] = b
	}
	n.AuthorID = userID(c)
	err := repo.QueryRow(`INSERT INTO evolution_notes(patient_id,appointment_id,subjective,objective,evaluation,plan,subjective_enc,objective_enc,evaluation_enc,plan_enc,author_id) VALUES($1,$2,'','','','',$3,$4,$5,$6,$7) RETURNING id,created_at`, n.PatientID, n.AppointmentID, enc[0], enc[1], enc[2], enc[3], n.AuthorID).Scan(&n.ID, &n.CreatedAt)
	if err != nil {
		return fail(c, 400, "No se pudo crear la nota")
	}
	audit(c, "crear", "nota_soep", strconv.FormatInt(n.ID, 10))
	return c.Status(201).JSON(fiber.Map{"id": n.ID, "created_at": n.CreatedAt, "signed": false})
}
func ListClinicalNotes(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	rows, err := repo.Query(`SELECT id,patient_id,appointment_id,subjective_enc,objective_enc,evaluation_enc,plan_enc,author_id,signed_at,digital_signature,created_at FROM evolution_notes WHERE patient_id=$1 ORDER BY created_at DESC LIMIT 300`, patientID)
	if err != nil {
		return fail(c, 500, "No se pudieron leer las notas")
	}
	defer rows.Close()
	out := []SOEPNote{}
	for rows.Next() {
		var n SOEPNote
		var appt sql.NullInt64
		var subj, obj, eval, plan []byte
		if rows.Scan(&n.ID, &n.PatientID, &appt, &subj, &obj, &eval, &plan, &n.AuthorID, &n.SignedAt, &n.Signature, &n.CreatedAt) != nil {
			continue
		}
		if appt.Valid {
			n.AppointmentID = &appt.Int64
		}
		for i, b := range [][]byte{subj, obj, eval, plan} {
			plain, e := dataCipher.Decrypt(b)
			if e != nil {
				return fail(c, 500, "No se pudo descifrar una nota clínica")
			}
			switch i {
			case 0:
				n.Subjective = string(plain)
			case 1:
				n.Objective = string(plain)
			case 2:
				n.Evaluation = string(plain)
			case 3:
				n.Plan = string(plain)
			}
		}
		out = append(out, n)
	}
	_ = rows.Close()
	addenda, err := repo.Query(`SELECT id,source_note_id,correction,correction_enc,signed_by,digital_signature,created_at FROM clinical_addenda WHERE patient_id=$1 ORDER BY created_at`, patientID)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar adendas")
	}
	defer addenda.Close()
	notesByID := make(map[int64]int, len(out))
	for i := range out {
		notesByID[out[i].ID] = i
		out[i].Addenda = []ClinicalAddendum{}
	}
	for addenda.Next() {
		var v ClinicalAddendum
		var source sql.NullInt64
		var encrypted []byte
		var legacy string
		if addenda.Scan(&v.ID, &source, &legacy, &encrypted, &v.SignedBy, &v.Signature, &v.CreatedAt) != nil || !source.Valid {
			continue
		}
		if len(encrypted) > 0 {
			plain, e := dataCipher.Decrypt(encrypted)
			if e != nil {
				return fail(c, 500, "No se pudo descifrar una adenda")
			}
			v.Correction = string(plain)
		} else {
			v.Correction = legacy
		}
		if idx, ok := notesByID[source.Int64]; ok {
			out[idx].Addenda = append(out[idx].Addenda, v)
		}
	}
	audit(c, "consultar", "nota_soep", strconv.FormatInt(patientID, 10))
	return c.JSON(out)
}
func UpdateClinicalNote(c *fiber.Ctx) error {
	id, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de nota inválido")
	}
	var n SOEPNote
	if c.BodyParser(&n) != nil || n.Subjective == "" || n.Objective == "" || n.Evaluation == "" || n.Plan == "" {
		return fail(c, 400, "Completa los cuatro apartados SOEP")
	}
	values := []string{n.Subjective, n.Objective, n.Evaluation, n.Plan}
	enc := make([][]byte, 4)
	for i, v := range values {
		enc[i], err = dataCipher.Encrypt([]byte(v))
		if err != nil {
			return fail(c, 500, "No se pudo proteger la nota")
		}
	}
	editHours, err := clinicalNoteEditHours()
	if err != nil {
		return fail(c, 500, err.Error())
	}
	res, err := repo.Exec(`UPDATE evolution_notes SET subjective='',objective='',evaluation='',plan='',subjective_enc=$1,objective_enc=$2,evaluation_enc=$3,plan_enc=$4 WHERE id=$5 AND author_id=$6 AND signed_at IS NULL AND created_at >= now()-($7::int*interval '1 hour')`, enc[0], enc[1], enc[2], enc[3], id, userID(c), editHours)
	if err != nil {
		return fail(c, 409, "La nota está bloqueada o no se puede modificar")
	}
	count, _ := res.RowsAffected()
	if count == 0 {
		return fail(c, 404, "Nota no encontrada, firmada o fuera del plazo de edición")
	}
	audit(c, "actualizar", "nota_soep", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"updated": true})
}
func SignClinicalNote(c *fiber.Ctx) error {
	id, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de nota inválido")
	}
	var n SOEPNote
	var enc [4][]byte
	if err = repo.QueryRow(`SELECT patient_id,subjective_enc,objective_enc,evaluation_enc,plan_enc FROM evolution_notes WHERE id=$1 AND author_id=$2 AND signed_at IS NULL`, id, userID(c)).Scan(&n.PatientID, &enc[0], &enc[1], &enc[2], &enc[3]); err != nil {
		return fail(c, 404, "Nota no disponible para firma")
	}
	payload := fmt.Sprintf("SIGO|SOEP|%d|%d|%d|%x|%x|%x|%x", id, n.PatientID, userID(c), enc[0], enc[1], enc[2], enc[3])
	signature, e := signUserMessage(userID(c), []byte(payload))
	if e != nil {
		return fail(c, 500, "No se pudo firmar la nota")
	}
	res, err := repo.Exec(`UPDATE evolution_notes SET signed_at=now(),digital_signature=$1 WHERE id=$2 AND author_id=$3 AND signed_at IS NULL`, signature, id, userID(c))
	if err != nil {
		return fail(c, 409, "La nota está bloqueada")
	}
	count, _ := res.RowsAffected()
	if count == 0 {
		return fail(c, 409, "La nota ya fue firmada")
	}
	audit(c, "firmar", "nota_soep", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"id": id, "signed": true, "signature": signature})
}
func AddClinicalAddendum(c *fiber.Ctx) error {
	id, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de nota inválido")
	}
	var x struct {
		Correction string `json:"correction"`
	}
	if c.BodyParser(&x) != nil || strings.TrimSpace(x.Correction) == "" {
		return fail(c, 400, "La adenda debe explicar la corrección")
	}
	var patientID int64
	if err = repo.QueryRow(`SELECT patient_id FROM evolution_notes WHERE id=$1 AND signed_at IS NOT NULL`, id).Scan(&patientID); err != nil {
		return fail(c, 404, "Nota firmada no encontrada")
	}
	message := fmt.Sprintf("SIGO|ADENDA|%d|%d|%d|%s", id, patientID, userID(c), x.Correction)
	signature, e := signUserMessage(userID(c), []byte(message))
	if e != nil {
		return fail(c, 500, "No se pudo firmar la adenda")
	}
	var addendumID int64
	correctionEnc, err := dataCipher.Encrypt([]byte(x.Correction))
	if err != nil {
		return fail(c, 500, "No se pudo proteger la adenda")
	}
	err = repo.QueryRow(`INSERT INTO clinical_addenda(patient_id,source_note_id,correction,correction_enc,signed_by,digital_signature) VALUES($1,$2,'',$3,$4,$5) RETURNING id`, patientID, id, correctionEnc, userID(c), signature).Scan(&addendumID)
	if err != nil {
		return fail(c, 500, "No se pudo guardar la adenda")
	}
	audit(c, "crear_adenda", "nota_soep", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": addendumID, "signed": true, "signature": signature})
}

func signUserMessage(userID int64, message []byte) (string, error) {
	tx, err := repo.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var public, privateEnc []byte
	if err = tx.QueryRow(`SELECT coalesce(signature_public_key,''::bytea),coalesce(signature_private_key_enc,''::bytea) FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&public, &privateEnc); err != nil {
		return "", err
	}
	if len(privateEnc) == 0 {
		pub, priv, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return "", e
		}
		privateEnc, e = dataCipher.Encrypt(priv)
		if e != nil {
			return "", e
		}
		public = pub
		if _, e = tx.Exec(`UPDATE users SET signature_public_key=$1,signature_private_key_enc=$2 WHERE id=$3`, public, privateEnc, userID); e != nil {
			return "", e
		}
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	private, err := dataCipher.Decrypt(privateEnc)
	if err != nil {
		return "", err
	}
	if err = security.ValidatePrivateKey(private); err != nil {
		return "", err
	}
	return security.SignEd25519(ed25519.PrivateKey(private), message), nil
}

func randomOTP() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	n := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return fmt.Sprintf("%06d", n%1000000), nil
}
func constantEqual(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
