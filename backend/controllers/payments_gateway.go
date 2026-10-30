package controllers

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"sigo-dental/backend/security"
)

// CreatePaymentIntent calls the clinic's configured gateway adapter. The adapter
// contract is documented in README.md and intentionally keeps card data out of SIGO.
func CreatePaymentIntent(c *fiber.Ctx) error {
	apiURL := strings.TrimSpace(os.Getenv("PAYMENT_API_URL"))
	apiToken := strings.TrimSpace(os.Getenv("PAYMENT_API_TOKEN"))
	webhookSecret := strings.TrimSpace(os.Getenv("PAYMENT_WEBHOOK_SECRET"))
	if apiURL == "" || apiToken == "" || webhookSecret == "" {
		return fail(c, 503, "La pasarela de pagos no está configurada")
	}
	u, err := url.ParseRequestURI(apiURL)
	if err != nil || (u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		return fail(c, 503, "La URL de la pasarela debe usar HTTPS")
	}
	id, err := parseID(c)
	if err != nil {
		return fail(c, 400, "ID inválido")
	}

	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo iniciar el cobro en línea")
	}
	defer tx.Rollback()
	var total float64
	var status string
	if err = tx.QueryRow(`SELECT total,status FROM invoices WHERE id=$1 FOR UPDATE`, id).Scan(&total, &status); err != nil {
		return fail(c, 404, "Cobro no encontrado")
	}
	if status == "anulada" || status == "pagada" {
		return fail(c, 409, "El cobro no tiene saldo pendiente")
	}
	var paid float64
	if err = tx.QueryRow(`SELECT coalesce(sum(amount),0) FROM payments WHERE invoice_id=$1`, id).Scan(&paid); err != nil {
		return fail(c, 500, "No se pudo consultar el saldo")
	}
	amount := math.Round((total-paid)*100) / 100
	if amount <= 0 {
		return fail(c, 409, "El cobro no tiene saldo pendiente")
	}
	var pendingReference, pendingURL string
	pendingErr := tx.QueryRow(`SELECT reference,checkout_url FROM payment_intents WHERE invoice_id=$1 AND status='pending' ORDER BY created_at DESC LIMIT 1`, id).Scan(&pendingReference, &pendingURL)
	if pendingErr == nil {
		if pendingURL == "" {
			return fail(c, 409, "Ya se está preparando un enlace de pago para esta factura")
		}
		if err = tx.Commit(); err != nil {
			return fail(c, 500, "No se pudo recuperar el enlace de pago")
		}
		return c.JSON(fiber.Map{"reference": pendingReference, "checkout_url": pendingURL, "amount": amount, "currency": "USD"})
	}
	if pendingErr != sql.ErrNoRows {
		return fail(c, 500, "No se pudieron consultar los intentos de pago")
	}
	var referenceBytes [16]byte
	if _, err = rand.Read(referenceBytes[:]); err != nil {
		return fail(c, 500, "No se pudo preparar el cobro seguro")
	}
	reference := "sigo-" + hex.EncodeToString(referenceBytes[:])
	if _, err = tx.Exec(`INSERT INTO payment_intents(invoice_id,reference,amount,currency) VALUES($1,$2,$3,'USD')`, id, reference, amount); err != nil {
		return fail(c, 500, "No se pudo preparar el cobro en línea")
	}
	if err = tx.Commit(); err != nil {
		return fail(c, 500, "No se pudo confirmar el cobro en línea")
	}

	payload, _ := json.Marshal(fiber.Map{
		"reference":   reference,
		"invoice_id":  id,
		"amount":      amount,
		"currency":    "USD",
		"description": fmt.Sprintf("SIGO Dental - factura #%d", id),
	})
	request, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		markPaymentIntentFailed(reference)
		return fail(c, 502, "No se pudo contactar a la pasarela")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiToken)
	request.Header.Set("Idempotency-Key", reference)
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		markPaymentIntentFailed(reference)
		return fail(c, 502, "La pasarela no respondió; el cobro no fue confirmado")
	}
	defer response.Body.Close()
	var gatewayResponse struct {
		CheckoutURL string `json:"checkout_url"`
		PaymentID   string `json:"payment_id"`
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&gatewayResponse) != nil || !validCheckoutURL(gatewayResponse.CheckoutURL) {
		markPaymentIntentFailed(reference)
		return fail(c, 502, "La pasarela no devolvió un enlace de pago válido")
	}
	if _, err = repo.Exec(`UPDATE payment_intents SET checkout_url=$1,provider_payment_id=$2,updated_at=now() WHERE reference=$3 AND status='pending'`, gatewayResponse.CheckoutURL, gatewayResponse.PaymentID, reference); err != nil {
		return fail(c, 500, "Se creó el intento, pero no se pudo guardar el enlace")
	}
	audit(c, "crear_intento_pasarela", "cobro", fmt.Sprint(id))
	return c.Status(201).JSON(fiber.Map{"reference": reference, "checkout_url": gatewayResponse.CheckoutURL, "amount": amount, "currency": "USD"})
}

func validCheckoutURL(value string) bool {
	u, err := url.ParseRequestURI(value)
	return err == nil && u.Host != "" && (u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1")))
}

func markPaymentIntentFailed(reference string) {
	_, _ = repo.Exec(`UPDATE payment_intents SET status='failed',updated_at=now() WHERE reference=$1 AND status='pending'`, reference)
}

// PaymentGatewayWebhook accepts only HMAC-signed events and records successful
// events idempotently before applying the payment in the same database transaction.
func PaymentGatewayWebhook(c *fiber.Ctx) error {
	secret := strings.TrimSpace(os.Getenv("PAYMENT_WEBHOOK_SECRET"))
	if secret == "" {
		return c.SendStatus(fiber.StatusServiceUnavailable)
	}
	provided := strings.TrimSpace(c.Get("X-Payment-Signature"))
	expected := security.Sign([]byte(secret), c.Body())
	if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	var event struct {
		ID        string  `json:"id"`
		Type      string  `json:"type"`
		Reference string  `json:"reference"`
		PaymentID string  `json:"payment_id"`
		Amount    float64 `json:"amount"`
		Currency  string  `json:"currency"`
	}
	if json.Unmarshal(c.Body(), &event) != nil || event.ID == "" || event.Reference == "" || (event.Type != "payment.succeeded" && event.Type != "payment.failed") {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	tx, err := repo.Begin()
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	defer tx.Rollback()
	var invoiceID int64
	var expectedAmount float64
	var currency, intentStatus string
	if err = tx.QueryRow(`SELECT invoice_id,amount,currency,status FROM payment_intents WHERE reference=$1 FOR UPDATE`, event.Reference).Scan(&invoiceID, &expectedAmount, &currency, &intentStatus); err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if math.Round(event.Amount*100) != math.Round(expectedAmount*100) || !strings.EqualFold(event.Currency, strings.TrimSpace(currency)) {
		return c.SendStatus(fiber.StatusConflict)
	}
	result, err := tx.Exec(`INSERT INTO payment_gateway_events(event_id,intent_reference,event_type) VALUES($1,$2,$3) ON CONFLICT(event_id) DO NOTHING`, event.ID, event.Reference, event.Type)
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if inserted == 0 {
		return c.SendStatus(fiber.StatusOK)
	}
	if event.Type == "payment.succeeded" && intentStatus != "succeeded" {
		var total float64
		var invoiceStatus string
		if err = tx.QueryRow(`SELECT total,status FROM invoices WHERE id=$1 FOR UPDATE`, invoiceID).Scan(&total, &invoiceStatus); err != nil {
			return c.SendStatus(fiber.StatusNotFound)
		}
		var paid float64
		if err = tx.QueryRow(`SELECT coalesce(sum(amount),0) FROM payments WHERE invoice_id=$1`, invoiceID).Scan(&paid); err != nil || invoiceStatus == "anulada" || paid+expectedAmount > total+0.001 {
			return c.SendStatus(fiber.StatusConflict)
		}
		if _, err = tx.Exec(`INSERT INTO payments(invoice_id,method,amount,reference) VALUES($1,'tarjeta',$2,$3)`, invoiceID, expectedAmount, "gateway:"+event.PaymentID); err != nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		if _, err = tx.Exec(`UPDATE payment_intents SET status='succeeded',provider_payment_id=COALESCE(NULLIF($1,''),provider_payment_id),updated_at=now() WHERE reference=$2`, event.PaymentID, event.Reference); err != nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		if paid+expectedAmount >= total-0.001 {
			if _, err = tx.Exec(`UPDATE invoices SET status='pagada' WHERE id=$1`, invoiceID); err != nil {
				return c.SendStatus(fiber.StatusInternalServerError)
			}
		}
	} else if event.Type == "payment.failed" && intentStatus == "pending" {
		if _, err = tx.Exec(`UPDATE payment_intents SET status='failed',provider_payment_id=COALESCE(NULLIF($1,''),provider_payment_id),updated_at=now() WHERE reference=$2`, event.PaymentID, event.Reference); err != nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
	}
	if _, err = tx.Exec(`INSERT INTO audit_log(action,entity,entity_id,ip_address) VALUES($1,'payment_gateway',$2,$3)`, event.Type, event.Reference, c.IP()); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if err = tx.Commit(); err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	return c.SendStatus(fiber.StatusOK)
}
