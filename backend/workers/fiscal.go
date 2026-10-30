package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"sigo-dental/backend/repository"
)

type fiscalJob struct {
	ID, InvoiceID int64
	Payload       []byte
	Attempts      int
}

// StartFiscal sends queued invoice documents to the clinic's configured fiscal provider adapter.
// The provider URL must be a certified issuer for the jurisdiction before production use.
func StartFiscal(ctx context.Context, repo repository.DBRepository) {
	endpoint, token := os.Getenv("FISCAL_API_URL"), os.Getenv("FISCAL_API_TOKEN")
	if endpoint == "" || token == "" {
		return
	}
	client := &http.Client{Timeout: 20 * time.Second}
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processFiscalBatch(repo, client, endpoint, token)
		}
	}
}

func processFiscalBatch(repo repository.DBRepository, client *http.Client, endpoint, token string) {
	tx, err := repo.Begin()
	if err != nil {
		return
	}
	rows, err := tx.Query(`SELECT id,invoice_id,request_payload,attempts FROM fiscal_jobs WHERE status IN ('pendiente','fallido') AND next_attempt_at<=now() AND attempts<6 ORDER BY created_at LIMIT 20 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		_ = tx.Rollback()
		return
	}
	batch := []fiscalJob{}
	for rows.Next() {
		var j fiscalJob
		if rows.Scan(&j.ID, &j.InvoiceID, &j.Payload, &j.Attempts) == nil {
			batch = append(batch, j)
		}
	}
	_ = rows.Close()
	for _, j := range batch {
		_, _ = tx.Exec(`UPDATE fiscal_jobs SET status='procesando',attempts=attempts+1,updated_at=now() WHERE id=$1`, j.ID)
	}
	if tx.Commit() != nil {
		return
	}
	for _, j := range batch {
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(j.Payload))
		if err != nil {
			retryFiscal(repo, j, err.Error())
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", fmt.Sprintf("sigo-invoice-%d", j.InvoiceID))
		resp, err := client.Do(req)
		if err != nil {
			retryFiscal(repo, j, err.Error())
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			retryFiscal(repo, j, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)))
			continue
		}
		var result struct {
			Reference string `json:"reference"`
			Status    string `json:"status"`
		}
		if json.Unmarshal(body, &result) != nil {
			retryFiscal(repo, j, "La respuesta fiscal no es JSON válido")
			continue
		}
		if result.Status != "emitido" && result.Status != "autorizado" && result.Status != "authorized" {
			retryFiscal(repo, j, "El proveedor no confirmó la emisión fiscal")
			continue
		}
		responseJSON := body
		_, _ = repo.Exec(`UPDATE fiscal_jobs SET status='emitido',response_payload=$1::jsonb,provider_reference=$2,updated_at=now() WHERE id=$3`, string(responseJSON), result.Reference, j.ID)
		_, _ = repo.Exec(`UPDATE invoices SET fiscal_status='emitido',fiscal_reference=$1,fiscal_response=$2::jsonb WHERE id=$3`, result.Reference, string(responseJSON), j.InvoiceID)
	}
}

func retryFiscal(repo repository.DBRepository, j fiscalJob, message string) {
	next := time.Now().Add(time.Duration(1<<min(j.Attempts, 7)) * time.Minute)
	status := "pendiente"
	if j.Attempts >= 5 {
		status = "fallido"
	}
	safe, _ := json.Marshal(map[string]string{"error": message})
	_, _ = repo.Exec(`UPDATE fiscal_jobs SET status=$1,response_payload=$2::jsonb,next_attempt_at=$3,updated_at=now() WHERE id=$4`, status, string(safe), next, j.ID)
	_, _ = repo.Exec(`UPDATE invoices SET fiscal_status=$1,fiscal_response=$2::jsonb WHERE id=$3`, status, string(safe), j.InvoiceID)
}
