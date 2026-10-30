package controllers

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

func CurrentCashSession(c *fiber.Ctx) error {
	var id int64
	var opening float64
	var opened time.Time
	err := repo.QueryRow(`SELECT id,opening_float,opened_at FROM cash_sessions WHERE user_id=$1 AND status='abierta'`, userID(c)).Scan(&id, &opening, &opened)
	if err != nil {
		return c.JSON(fiber.Map{"open": false})
	}
	// Deliberately omit expected receipts until the physical count is submitted.
	audit(c, "consultar", "caja_abierta", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"open": true, "id": id, "opening_float": opening, "opened_at": opened})
}

func OpenCashSession(c *fiber.Ctx) error {
	var x struct {
		OpeningFloat float64 `json:"opening_float"`
	}
	if c.BodyParser(&x) != nil || x.OpeningFloat < 0 {
		return fail(c, 400, "El fondo inicial debe ser válido")
	}
	var id int64
	err := repo.QueryRow(`INSERT INTO cash_sessions(user_id,opening_float) VALUES($1,$2) RETURNING id`, userID(c), x.OpeningFloat).Scan(&id)
	if err != nil {
		return fail(c, 409, "Ya existe una caja abierta o no se pudo abrir")
	}
	audit(c, "abrir", "caja", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id, "status": "abierta"})
}

func CloseCashSession(c *fiber.Ctx) error {
	var x struct {
		PhysicalCount float64 `json:"physical_count"`
	}
	if c.BodyParser(&x) != nil || x.PhysicalCount < 0 {
		return fail(c, 400, "El conteo físico debe ser válido")
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo cerrar la caja")
	}
	defer tx.Rollback()
	var id int64
	var opening float64
	if err = tx.QueryRow(`SELECT id,opening_float FROM cash_sessions WHERE user_id=$1 AND status='abierta' FOR UPDATE`, userID(c)).Scan(&id, &opening); err != nil {
		return fail(c, 404, "No tienes una caja abierta")
	}
	var cashIn float64
	if err = tx.QueryRow(`SELECT coalesce(sum(p.amount),0) FROM payments p WHERE p.cash_session_id=$1 AND p.method='efectivo'`, id).Scan(&cashIn); err != nil {
		return fail(c, 500, "No se pudo calcular la caja")
	}
	expected := opening + cashIn
	variance := x.PhysicalCount - expected
	_, err = tx.Exec(`UPDATE cash_sessions SET closed_at=now(),physical_count=$1,expected_cash=$2,variance=$3,status='cerrada' WHERE id=$4`, x.PhysicalCount, expected, variance, id)
	if err != nil {
		return fail(c, 500, "No se pudo guardar el cierre")
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar el cierre")
	}
	audit(c, "cerrar", "caja", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"id": id, "status": "cerrada", "physical_count": x.PhysicalCount, "expected_cash": expected, "variance": variance})
}

func CashReport(c *fiber.Ctx) error {
	// Only administrators and auditors are routed here. A closed session exposes reconciliation values.
	rows, err := repo.Query(`SELECT id,user_id,opened_at,closed_at,opening_float,physical_count,expected_cash,variance,status FROM cash_sessions ORDER BY opened_at DESC LIMIT 100`)
	if err != nil {
		return fail(c, 500, "No se pudo consultar el reporte de caja")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id, uid int64
		var opened time.Time
		var closed sql.NullTime
		var opening float64
		var physical, expected, variance sql.NullFloat64
		var status string
		if rows.Scan(&id, &uid, &opened, &closed, &opening, &physical, &expected, &variance, &status) != nil {
			continue
		}
		entry := fiber.Map{"id": id, "user_id": uid, "opened_at": opened, "opening_float": opening, "status": status}
		if closed.Valid {
			entry["closed_at"] = closed.Time
			entry["physical_count"] = physical.Float64
			entry["expected_cash"] = expected.Float64
			entry["variance"] = variance.Float64
		}
		out = append(out, entry)
	}
	return c.JSON(out)
}
