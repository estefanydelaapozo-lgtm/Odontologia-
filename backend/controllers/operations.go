package controllers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"sigo-dental/backend/domain"
)

func ListInventory(c *fiber.Ctx) error {
	rows, err := repo.Query(`SELECT i.id,i.sku,i.name,i.unit,i.minimum_stock,i.unit_cost,i.active,coalesce(sum(b.quantity),0) FROM inventory_items i LEFT JOIN inventory_balances b ON b.item_id=i.id GROUP BY i.id ORDER BY i.name`)
	if err != nil {
		return fail(c, 500, "No se pudo consultar inventario")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id int64
		var sku, name, unit string
		var minimum, cost, stock float64
		var active bool
		if rows.Scan(&id, &sku, &name, &unit, &minimum, &cost, &active, &stock) == nil {
			out = append(out, fiber.Map{"id": id, "sku": sku, "name": name, "unit": unit, "minimum_stock": minimum, "unit_cost": cost, "active": active, "stock": stock, "low_stock": stock <= minimum})
		}
	}
	audit(c, "consultar", "inventario", "listado")
	return c.JSON(out)
}
func CreateInventoryItem(c *fiber.Ctx) error {
	var x struct {
		SKU      string  `json:"sku"`
		Name     string  `json:"name"`
		Unit     string  `json:"unit"`
		Minimum  float64 `json:"minimum_stock"`
		Cost     float64 `json:"unit_cost"`
		Initial  float64 `json:"initial_stock"`
		Location string  `json:"location"`
		ChairID  int64   `json:"chair_id"`
	}
	if c.BodyParser(&x) != nil || x.SKU == "" || x.Name == "" || x.Minimum < 0 || x.Cost < 0 || x.Initial < 0 {
		return fail(c, 400, "SKU, nombre y valores de inventario válidos son obligatorios")
	}
	if x.Unit == "" {
		x.Unit = "unidad"
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo crear el producto")
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRow(`INSERT INTO inventory_items(sku,name,unit,minimum_stock,unit_cost) VALUES($1,$2,$3,$4,$5) RETURNING id`, x.SKU, x.Name, x.Unit, x.Minimum, x.Cost).Scan(&id)
	if err != nil {
		return fail(c, 409, "SKU ya existe o producto inválido")
	}
	if x.Initial > 0 {
		_, err = tx.Exec(`INSERT INTO inventory_balances(item_id,chair_id,location,quantity) VALUES($1,NULLIF($2,0),$3,$4)`, id, x.ChairID, x.Location, x.Initial)
		if err != nil {
			return fail(c, 400, "Ubicación de inventario inválida")
		}
		_, err = tx.Exec(`INSERT INTO inventory_movements(item_id,chair_id,movement_type,quantity,reference_type,reference_id,recorded_by) VALUES($1,NULLIF($2,0),'entrada','0'::numeric+$3,'initial_stock',$1,$4)`, id, x.ChairID, x.Initial, userID(c))
		if err != nil {
			return fail(c, 500, "No se pudo registrar el ingreso inicial")
		}
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar el producto")
	}
	audit(c, "crear", "inventario", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id})
}
func AdjustInventory(c *fiber.Ctx) error {
	var x struct {
		ItemID, ChairID int64
		Quantity        float64
		Movement        string
		Location        string
	}
	var body struct {
		ItemID   int64   `json:"item_id"`
		ChairID  int64   `json:"chair_id"`
		Quantity float64 `json:"quantity"`
		Movement string  `json:"movement_type"`
		Location string  `json:"location"`
	}
	if c.BodyParser(&body) != nil || body.ItemID < 1 || body.Quantity <= 0 || (body.Movement != "entrada" && body.Movement != "ajuste" && body.Movement != "devolucion") {
		return fail(c, 400, "Movimiento de inventario inválido")
	}
	x.ItemID = body.ItemID
	x.ChairID = body.ChairID
	x.Quantity = body.Quantity
	x.Movement = body.Movement
	x.Location = body.Location
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo iniciar el movimiento")
	}
	defer tx.Rollback()
	var balanceID int64
	var stock float64
	err = tx.QueryRow(`SELECT id,quantity FROM inventory_balances WHERE item_id=$1 AND chair_id IS NOT DISTINCT FROM NULLIF($2,0) AND location=$3 FOR UPDATE`, x.ItemID, x.ChairID, x.Location).Scan(&balanceID, &stock)
	if err != nil {
		if x.Movement != "entrada" {
			return fail(c, 404, "Ubicación de inventario no encontrada")
		}
		err = tx.QueryRow(`INSERT INTO inventory_balances(item_id,chair_id,location,quantity) VALUES($1,NULLIF($2,0),$3,0) RETURNING id,quantity`, x.ItemID, x.ChairID, x.Location).Scan(&balanceID, &stock)
		if err != nil {
			return fail(c, 400, "No se pudo crear ubicación")
		}
	}
	newStock := stock + x.Quantity
	if x.Movement == "ajuste" {
		newStock = x.Quantity
	}
	if _, err = tx.Exec(`UPDATE inventory_balances SET quantity=$1 WHERE id=$2`, newStock, balanceID); err != nil {
		return fail(c, 409, "El inventario no puede quedar negativo")
	}
	if _, err = tx.Exec(`INSERT INTO inventory_movements(item_id,chair_id,movement_type,quantity,reference_type,reference_id,recorded_by) VALUES($1,NULLIF($2,0),$3,$4,'manual',$1,$5)`, x.ItemID, x.ChairID, x.Movement, x.Quantity, userID(c)); err != nil {
		return fail(c, 500, "No se pudo guardar movimiento")
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar movimiento")
	}
	audit(c, x.Movement, "inventario", strconv.FormatInt(x.ItemID, 10))
	return c.JSON(fiber.Map{"item_id": x.ItemID, "stock": newStock})
}
func SetServiceMaterials(c *fiber.Ctx) error {
	serviceID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de servicio inválido")
	}
	var x struct {
		Materials []struct {
			ItemID   int64   `json:"item_id"`
			Quantity float64 `json:"quantity"`
		} `json:"materials"`
	}
	if c.BodyParser(&x) != nil {
		return fail(c, 400, "Receta de materiales inválida")
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo guardar receta")
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM service_materials WHERE service_id=$1`, serviceID); err != nil {
		return fail(c, 500, "No se pudo reemplazar la receta")
	}
	for _, m := range x.Materials {
		if m.ItemID < 1 || m.Quantity <= 0 {
			return fail(c, 400, "Material y cantidad deben ser válidos")
		}
		if _, err = tx.Exec(`INSERT INTO service_materials(service_id,item_id,quantity) VALUES($1,$2,$3)`, serviceID, m.ItemID, m.Quantity); err != nil {
			return fail(c, 400, "Material inválido o repetido")
		}
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar receta")
	}
	audit(c, "actualizar_receta", "servicio", strconv.FormatInt(serviceID, 10))
	return c.JSON(fiber.Map{"materials": len(x.Materials)})
}

func ListServiceMaterials(c *fiber.Ctx) error {
	serviceID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de servicio inválido")
	}
	rows, err := repo.Query(`SELECT sm.item_id,i.name,sm.quantity FROM service_materials sm JOIN inventory_items i ON i.id=sm.item_id WHERE sm.service_id=$1 ORDER BY i.name`, serviceID)
	if err != nil {
		return fail(c, 500, "No se pudo consultar receta")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id int64
		var name string
		var qty float64
		if rows.Scan(&id, &name, &qty) == nil {
			out = append(out, fiber.Map{"item_id": id, "name": name, "quantity": qty})
		}
	}
	return c.JSON(out)
}

func CreateFeeRule(c *fiber.Ctx) error {
	var x struct {
		ProfessionalID int64   `json:"professional_id"`
		ServiceID      *int64  `json:"service_id"`
		Type           string  `json:"rule_type"`
		Rate           float64 `json:"rate"`
	}
	if c.BodyParser(&x) != nil || x.ProfessionalID < 1 || x.Rate < 0 || ((x.Type == "porcentaje" || x.Type == "porcentaje_neto") && x.Rate > 100) || (x.Type != "porcentaje" && x.Type != "fijo" && x.Type != "porcentaje_neto") {
		return fail(c, 400, "Regla de honorarios inválida")
	}
	var id int64
	err := repo.QueryRow(`INSERT INTO professional_fee_rules(professional_id,service_id,rule_type,rate) VALUES($1,$2,$3,$4) RETURNING id`, x.ProfessionalID, x.ServiceID, x.Type, x.Rate).Scan(&id)
	if err != nil {
		return fail(c, 400, "No se pudo guardar regla")
	}
	audit(c, "crear", "regla_honorarios", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id})
}
func CreateSettlement(c *fiber.Ctx) error {
	var x struct {
		ProfessionalID int64  `json:"professional_id"`
		Start          string `json:"period_start"`
		End            string `json:"period_end"`
	}
	if c.BodyParser(&x) != nil || x.ProfessionalID < 1 {
		return fail(c, 400, "Profesional y periodo son obligatorios")
	}
	start, e1 := time.Parse("2006-01-02", x.Start)
	end, e2 := time.Parse("2006-01-02", x.End)
	if e1 != nil || e2 != nil || end.Before(start) {
		return fail(c, 400, "Periodo inválido")
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo liquidar")
	}
	defer tx.Rollback()
	var sid int64
	err = tx.QueryRow(`INSERT INTO professional_settlements(professional_id,period_start,period_end,gross,deductions,net,created_by) VALUES($1,$2,$3,0,0,0,$4) RETURNING id`, x.ProfessionalID, start, end, userID(c)).Scan(&sid)
	if err != nil {
		return fail(c, 409, "Ya existe una liquidación para ese periodo")
	}
	rows, err := tx.Query(`SELECT tp.id,tp.service_id,coalesce(tpi.professional_fee-tpi.discount,s.price),coalesce(tpi.laboratory_cost,0),coalesce((SELECT sum(sm.quantity*ii.unit_cost) FROM service_materials sm JOIN inventory_items ii ON ii.id=sm.item_id WHERE sm.service_id=tp.service_id),0),coalesce(r.rule_type,'porcentaje'),coalesce(r.rate,100) FROM treatment_performances tp LEFT JOIN treatment_plan_items tpi ON tpi.id=tp.plan_item_id JOIN services s ON s.id=tp.service_id LEFT JOIN LATERAL (SELECT rule_type,rate FROM professional_fee_rules WHERE professional_id=tp.professional_id AND (service_id=tp.service_id OR service_id IS NULL) AND active=TRUE AND valid_from<=tp.performed_at::date ORDER BY (service_id IS NOT NULL) DESC,valid_from DESC,id DESC LIMIT 1) r ON TRUE WHERE tp.professional_id=$1 AND tp.performed_at::date BETWEEN $2 AND $3 AND NOT EXISTS(SELECT 1 FROM professional_settlement_items i WHERE i.treatment_id=tp.id) ORDER BY tp.id`, x.ProfessionalID, start, end)
	if err != nil {
		return fail(c, 500, "No se pudieron reunir procedimientos")
	}
	type item struct {
		id, service                 int64
		gross, lab, materials, rate float64
		rule                        string
	}
	items := []item{}
	for rows.Next() {
		var v item
		if rows.Scan(&v.id, &v.service, &v.gross, &v.lab, &v.materials, &v.rule, &v.rate) == nil {
			items = append(items, v)
		}
	}
	_ = rows.Close()
	var grossSum, netSum float64
	for _, v := range items {
		net := domain.CalculateProfessionalFee(v.gross, v.lab, v.materials, domain.FeeRule{Type: v.rule, Rate: v.rate})
		deduction := v.gross - net
		grossSum += v.gross
		netSum += net
		if _, err = tx.Exec(`INSERT INTO professional_settlement_items(settlement_id,treatment_id,service_id,gross,deductions,net) VALUES($1,$2,$3,$4,$5,$6)`, sid, v.id, v.service, v.gross, deduction, net); err != nil {
			return fail(c, 500, "No se pudo agregar procedimiento")
		}
	}
	if _, err = tx.Exec(`UPDATE professional_settlements SET gross=$1,deductions=$2,net=$3 WHERE id=$4`, grossSum, grossSum-netSum, netSum, sid); err != nil {
		return fail(c, 500, "No se pudo calcular liquidación")
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar liquidación")
	}
	audit(c, "crear", "liquidacion_honorarios", strconv.FormatInt(sid, 10))
	return c.Status(201).JSON(fiber.Map{"id": sid, "gross": grossSum, "deductions": grossSum - netSum, "net": netSum, "items": len(items)})
}
func ListSettlements(c *fiber.Ctx) error {
	rows, err := repo.Query(`SELECT s.id,s.professional_id,p.full_name,s.period_start,s.period_end,s.gross,s.deductions,s.net,s.status FROM professional_settlements s JOIN professionals p ON p.id=s.professional_id ORDER BY s.period_start DESC LIMIT 200`)
	if err != nil {
		return fail(c, 500, "No se pudo consultar honorarios")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id, pid int64
		var name, status string
		var start, end time.Time
		var gross, deductions, net float64
		if rows.Scan(&id, &pid, &name, &start, &end, &gross, &deductions, &net, &status) == nil {
			out = append(out, fiber.Map{"id": id, "professional_id": pid, "professional": name, "period_start": start, "period_end": end, "gross": gross, "deductions": deductions, "net": net, "status": status})
		}
	}
	return c.JSON(out)
}
