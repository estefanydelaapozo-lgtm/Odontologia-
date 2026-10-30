package controllers

import (
	"database/sql"
	"encoding/json"
	"os"
	"time"
)

func queueFiscalInvoice(tx *sql.Tx, invoiceID, patientID int64, total float64) error {
	if os.Getenv("FISCAL_API_URL") == "" || os.Getenv("FISCAL_API_TOKEN") == "" {
		return nil
	}
	var document, name string
	if err := tx.QueryRow(`SELECT coalesce(document_id,''),first_name||' '||last_name FROM patients WHERE id=$1`, patientID).Scan(&document, &name); err != nil {
		return err
	}
	var subtotal, discount float64
	if err := tx.QueryRow(`SELECT subtotal,discount FROM invoices WHERE id=$1`, invoiceID).Scan(&subtotal, &discount); err != nil {
		return err
	}
	clinic := struct {
		Name  string `json:"name"`
		TaxID string `json:"tax_id"`
	}{}
	_ = tx.QueryRow(`SELECT name,tax_id FROM clinics WHERE active=TRUE ORDER BY id LIMIT 1`).Scan(&clinic.Name, &clinic.TaxID)
	type fiscalLine struct {
		Description string  `json:"description"`
		Quantity    float64 `json:"quantity"`
		UnitPrice   float64 `json:"unit_price"`
		Discount    float64 `json:"discount"`
		Total       float64 `json:"total"`
	}
	lines := []fiscalLine{}
	rows, err := tx.Query(`SELECT description,quantity,unit_price,discount,total FROM invoice_items WHERE invoice_id=$1 ORDER BY id`, invoiceID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var line fiscalLine
		if rows.Scan(&line.Description, &line.Quantity, &line.UnitPrice, &line.Discount, &line.Total) == nil {
			lines = append(lines, line)
		}
	}
	rows.Close()
	if len(lines) == 0 {
		lines = append(lines, fiscalLine{Description: "Servicios odontológicos", Quantity: 1, UnitPrice: subtotal, Total: subtotal})
	}
	payload, err := json.Marshal(map[string]any{"invoice_id": invoiceID, "clinic": clinic, "patient": map[string]string{"document_id": document, "name": name}, "items": lines, "currency": "USD", "subtotal": subtotal, "discount": discount, "total": total, "issued_at": time.Now().UTC()})
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE invoices SET fiscal_status='pendiente' WHERE id=$1`, invoiceID); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO fiscal_jobs(invoice_id,provider,status,request_payload) VALUES($1,'http_adapter','pendiente',$2::jsonb)`, invoiceID, string(payload))
	return err
}
