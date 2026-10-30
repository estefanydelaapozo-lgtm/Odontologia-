package controllers

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lib/pq"
)

type planItemInput struct {
	ServiceID   int64   `json:"service_id"`
	Phase       string  `json:"phase_name"`
	Session     int     `json:"session_number"`
	Description string  `json:"description"`
	Fee         float64 `json:"professional_fee"`
	Lab         float64 `json:"laboratory_cost"`
	Discount    float64 `json:"discount"`
	Quantity    int     `json:"quantity"`
}

func ListTreatmentPlans(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	rows, err := repo.Query(`SELECT p.id,p.title,p.status,p.created_at,i.id,i.service_id,s.name,i.phase_name,i.session_number,i.description,i.professional_fee,i.laboratory_cost,i.discount,i.quantity,i.approved_quantity,i.status FROM treatment_plans p LEFT JOIN treatment_plan_items i ON i.plan_id=p.id LEFT JOIN services s ON s.id=i.service_id WHERE p.patient_id=$1 ORDER BY p.created_at DESC,i.phase_name,i.session_number,i.id`, patientID)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar planes")
	}
	defer rows.Close()
	plans := map[int64]fiber.Map{}
	order := []int64{}
	for rows.Next() {
		var pid int64
		var title, status string
		var created time.Time
		var itemID, serviceID *int64
		var serviceName, phase, description, itemStatus *string
		var session, quantity, approved *int
		var fee, lab, discount *float64
		if rows.Scan(&pid, &title, &status, &created, &itemID, &serviceID, &serviceName, &phase, &session, &description, &fee, &lab, &discount, &quantity, &approved, &itemStatus) != nil {
			continue
		}
		p, ok := plans[pid]
		if !ok {
			p = fiber.Map{"id": pid, "title": title, "status": status, "created_at": created, "items": []fiber.Map{}}
			plans[pid] = p
			order = append(order, pid)
		}
		if itemID != nil {
			items := p["items"].([]fiber.Map)
			items = append(items, fiber.Map{"id": *itemID, "service_id": *serviceID, "service_name": *serviceName, "phase_name": *phase, "session_number": *session, "description": *description, "professional_fee": *fee, "laboratory_cost": *lab, "discount": *discount, "quantity": *quantity, "approved_quantity": *approved, "status": *itemStatus})
			p["items"] = items
		}
	}
	out := []fiber.Map{}
	for _, id := range order {
		out = append(out, plans[id])
	}
	audit(c, "consultar", "planes_tratamiento", strconv.FormatInt(patientID, 10))
	return c.JSON(out)
}

func CreateTreatmentPlan(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	var x struct {
		Title string          `json:"title"`
		Items []planItemInput `json:"items"`
	}
	if c.BodyParser(&x) != nil || strings.TrimSpace(x.Title) == "" || len(x.Items) == 0 {
		return fail(c, 400, "Indica el nombre del plan y al menos un procedimiento")
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo iniciar el plan")
	}
	defer tx.Rollback()
	var planID int64
	if err = tx.QueryRow(`INSERT INTO treatment_plans(patient_id,title,created_by) VALUES($1,$2,$3) RETURNING id`, patientID, strings.TrimSpace(x.Title), userID(c)).Scan(&planID); err != nil {
		return fail(c, 400, "No se pudo guardar el plan")
	}
	for _, item := range x.Items {
		if item.ServiceID < 1 || item.Fee < 0 || item.Lab < 0 || item.Discount < 0 || item.Discount > item.Fee || item.Quantity < 1 || item.Session < 1 {
			return fail(c, 400, "Revisa precios, descuentos, cantidades y sesiones del plan")
		}
		if item.Discount > 0 && c.Locals("role") != "admin" {
			return fail(c, 403, "Solo administración puede autorizar descuentos")
		}
		if item.Phase == "" {
			item.Phase = "Fase 1"
		}
		if _, err = tx.Exec(`INSERT INTO treatment_plan_items(plan_id,service_id,phase_name,session_number,description,professional_fee,laboratory_cost,discount,discount_authorized_by,quantity) VALUES($1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $8>0 THEN $9 ELSE NULL END,$10)`, planID, item.ServiceID, item.Phase, item.Session, item.Description, item.Fee, item.Lab, item.Discount, userID(c), item.Quantity); err != nil {
			return fail(c, 400, "Un procedimiento del plan no es válido")
		}
	}
	if _, err = tx.Exec(`UPDATE treatment_plans SET status='presentado' WHERE id=$1`, planID); err != nil {
		return fail(c, 500, "No se pudo presentar el plan")
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar el plan")
	}
	audit(c, "crear", "plan_tratamiento", strconv.FormatInt(planID, 10))
	return c.Status(201).JSON(fiber.Map{"id": planID, "status": "presentado"})
}

func ApproveTreatmentPlan(c *fiber.Ctx) error {
	planID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de plan inválido")
	}
	var x struct {
		ItemIDs   []int64 `json:"item_ids"`
		Method    string  `json:"method"`
		Signature string  `json:"signature"`
	}
	if c.BodyParser(&x) != nil || len(x.ItemIDs) == 0 || x.Method != "presencial" {
		return fail(c, 400, "Selecciona procedimientos y registra la firma presencial del paciente")
	}
	var patientID int64
	if err = repo.QueryRow(`SELECT patient_id FROM treatment_plans WHERE id=$1 AND status IN ('presentado','aprobado_parcial')`, planID).Scan(&patientID); err != nil {
		return fail(c, 404, "Plan no disponible para aprobación")
	}
	if len(strings.TrimSpace(x.Signature)) < 20 {
		return fail(c, 400, "Captura la firma del paciente")
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo guardar la aprobación")
	}
	defer tx.Rollback()
	var approvedCount int
	if err = tx.QueryRow(`SELECT count(*) FROM treatment_plan_items WHERE plan_id=$1 AND id=ANY($2) AND status='propuesto'`, planID, pq.Array(x.ItemIDs)).Scan(&approvedCount); err != nil || approvedCount != len(x.ItemIDs) {
		return fail(c, 409, "Hay procedimientos repetidos o que no pertenecen al plan")
	}
	for _, itemID := range x.ItemIDs {
		if _, err = tx.Exec(`UPDATE treatment_plan_items SET approved_quantity=quantity,status='aprobado' WHERE plan_id=$1 AND id=$2 AND status='propuesto'`, planID, itemID); err != nil {
			return fail(c, 500, "No se pudo aprobar el procedimiento")
		}
	}
	enc, e := dataCipher.Encrypt([]byte(x.Signature))
	if e != nil {
		return fail(c, 500, "No se pudo proteger la firma")
	}
	_, err = tx.Exec(`INSERT INTO treatment_approvals(plan_id,patient_id,approval_type,approved_item_ids,approval_method,signature_data,recorded_by) VALUES($1,$2,'parcial',$3,'presencial',$4,$5)`, planID, patientID, pq.Array(x.ItemIDs), base64.StdEncoding.EncodeToString(enc), userID(c))
	if err != nil {
		return fail(c, 500, "No se pudo registrar la aprobación")
	}
	var pending int
	_ = tx.QueryRow(`SELECT count(*) FROM treatment_plan_items WHERE plan_id=$1 AND status='propuesto'`, planID).Scan(&pending)
	status := "aprobado"
	if pending > 0 {
		status = "aprobado_parcial"
	}
	_, err = tx.Exec(`UPDATE treatment_plans SET status=$1,approved_at=now() WHERE id=$2`, status, planID)
	if err != nil {
		return fail(c, 500, "No se pudo actualizar el plan")
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar la aprobación")
	}
	audit(c, "aprobar_parcial", "plan_tratamiento", strconv.FormatInt(planID, 10))
	return c.JSON(fiber.Map{"plan_id": planID, "status": status, "approved_item_ids": x.ItemIDs})
}

func PerformTreatment(c *fiber.Ctx) error {
	var x struct {
		PatientID      int64  `json:"patient_id"`
		ServiceID      int64  `json:"service_id"`
		PlanItemID     int64  `json:"plan_item_id"`
		AppointmentID  int64  `json:"appointment_id"`
		ProfessionalID int64  `json:"professional_id"`
		ChairID        int64  `json:"chair_id"`
		Notes          string `json:"notes"`
	}
	if c.BodyParser(&x) != nil || x.PatientID < 1 || x.ProfessionalID < 1 {
		return fail(c, 400, "Paciente y profesional son obligatorios")
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo registrar el procedimiento")
	}
	defer tx.Rollback()
	var serviceID int64
	var fee, laboratoryCost float64
	if x.PlanItemID > 0 {
		err = tx.QueryRow(`SELECT service_id,professional_fee-discount,laboratory_cost FROM treatment_plan_items WHERE id=$1 AND plan_id IN (SELECT id FROM treatment_plans WHERE patient_id=$2) AND approved_quantity>0 AND status IN ('aprobado','agendado') FOR UPDATE`, x.PlanItemID, x.PatientID).Scan(&serviceID, &fee, &laboratoryCost)
	} else {
		err = tx.QueryRow(`SELECT id,price FROM services WHERE id=$1 AND active=TRUE`, x.ServiceID).Scan(&serviceID, &fee)
	}
	if err != nil {
		return fail(c, 409, "El servicio o procedimiento no fue aprobado")
	}
	if x.AppointmentID > 0 {
		var appointmentPatient, appointmentItem, appointmentService int64
		var appointmentStatus string
		if err = tx.QueryRow(`SELECT patient_id,status,coalesce(treatment_item_id,0),coalesce(service_id,0) FROM appointments WHERE id=$1 FOR UPDATE`, x.AppointmentID).Scan(&appointmentPatient, &appointmentStatus, &appointmentItem, &appointmentService); err != nil || appointmentPatient != x.PatientID || appointmentStatus != "en_atencion" || (x.PlanItemID > 0 && appointmentItem != x.PlanItemID) || (appointmentService > 0 && appointmentService != serviceID) {
			return fail(c, 409, "La cita debe estar en atención y corresponder al procedimiento")
		}
	}
	var notesEnc []byte
	if x.Notes != "" {
		notesEnc, err = dataCipher.Encrypt([]byte(x.Notes))
		if err != nil {
			return fail(c, 500, "No se pudieron proteger las notas")
		}
	}
	sig, e := signUserMessage(userID(c), []byte(strconv.FormatInt(x.PatientID, 10)+"|"+strconv.FormatInt(serviceID, 10)+"|"+time.Now().UTC().Format(time.RFC3339Nano)))
	if e != nil {
		return fail(c, 500, "No se pudo firmar el procedimiento")
	}
	var id int64
	err = tx.QueryRow(`INSERT INTO treatment_performances(patient_id,plan_item_id,appointment_id,service_id,professional_id,performed_by,digital_signature,notes,notes_enc) VALUES($1,NULLIF($2,0),NULLIF($3,0),$4,$5,$6,$7,'',$8) RETURNING id`, x.PatientID, x.PlanItemID, x.AppointmentID, serviceID, x.ProfessionalID, userID(c), sig, notesEnc).Scan(&id)
	if err != nil {
		return fail(c, 409, "El procedimiento ya fue registrado o no es válido")
	}
	if x.PlanItemID > 0 {
		_, err = tx.Exec(`UPDATE treatment_plan_items SET status='realizado' WHERE id=$1`, x.PlanItemID)
		if err != nil {
			return fail(c, 500, "No se pudo actualizar el plan")
		}
	}
	if x.AppointmentID > 0 {
		_, err = tx.Exec(`UPDATE appointments SET status='atendido' WHERE id=$1`, x.AppointmentID)
		if err != nil {
			return fail(c, 500, "No se pudo cerrar la cita")
		}
	}
	rows, err := tx.Query(`SELECT item_id,quantity FROM service_materials WHERE service_id=$1`, serviceID)
	if err != nil {
		return fail(c, 500, "No se pudo revisar el inventario")
	}
	type requiredMaterial struct {
		item int64
		qty  float64
	}
	needed := []requiredMaterial{}
	for rows.Next() {
		var v requiredMaterial
		if rows.Scan(&v.item, &v.qty) == nil {
			needed = append(needed, v)
		}
	}
	_ = rows.Close()
	type consumption struct {
		item, balance, chair int64
		qty                  float64
	}
	items := []consumption{}
	for _, material := range needed {
		balances, queryErr := tx.Query(`SELECT id,coalesce(chair_id,0),quantity FROM inventory_balances WHERE item_id=$1 AND (chair_id IS NULL OR chair_id=$2) ORDER BY (chair_id IS NOT NULL) DESC,id FOR UPDATE`, material.item, x.ChairID)
		if queryErr != nil {
			return fail(c, 500, "No se pudo bloquear inventario")
		}
		remaining := material.qty
		for balances.Next() && remaining > 0 {
			var balanceID, chairID int64
			var stock float64
			if balances.Scan(&balanceID, &chairID, &stock) != nil || stock <= 0 {
				continue
			}
			take := remaining
			if stock < take {
				take = stock
			}
			items = append(items, consumption{item: material.item, balance: balanceID, chair: chairID, qty: take})
			remaining -= take
		}
		_ = balances.Close()
		if remaining > 0 {
			return fail(c, 409, "Inventario insuficiente para realizar el procedimiento")
		}
	}
	for _, v := range items {
		if _, err = tx.Exec(`UPDATE inventory_balances SET quantity=quantity-$1 WHERE id=$2`, v.qty, v.balance); err != nil {
			return fail(c, 500, "No se pudo descontar inventario")
		}
		if _, err = tx.Exec(`INSERT INTO inventory_movements(item_id,chair_id,movement_type,quantity,reference_type,reference_id,recorded_by) VALUES($1,NULLIF($2,0),'consumo',$3,'treatment_performance',$4,$5)`, v.item, v.chair, v.qty, id, userID(c)); err != nil {
			return fail(c, 500, "No se pudo registrar el movimiento de inventario")
		}
	}
	// Create an invoice from the completed service in the same transaction.
	var invoiceID int64
	patientAmount := fee + laboratoryCost
	err = tx.QueryRow(`INSERT INTO invoices(patient_id,appointment_id,subtotal,discount,total,created_by) VALUES($1,NULLIF($2,0),$3,0,$3,$4) RETURNING id`, x.PatientID, x.AppointmentID, patientAmount, userID(c)).Scan(&invoiceID)
	if err != nil {
		return fail(c, 500, "No se pudo generar el cobro del procedimiento")
	}
	_, err = tx.Exec(`INSERT INTO invoice_items(invoice_id,description,quantity,unit_price,total,service_id,treatment_item_id) SELECT $1,name,1,$2,$2,$3,NULLIF($4,0) FROM services WHERE id=$3`, invoiceID, patientAmount, serviceID, x.PlanItemID)
	if err != nil {
		return fail(c, 500, "No se pudo detallar el cobro")
	}
	if err = queueFiscalInvoice(tx, invoiceID, x.PatientID, patientAmount); err != nil {
		return fail(c, 500, "No se pudo preparar la emisión fiscal")
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar el procedimiento")
	}
	audit(c, "realizar", "procedimiento", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id, "invoice_id": invoiceID, "signed": true})
}
