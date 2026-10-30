package workers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"sigo-dental/backend/repository"
	"sigo-dental/backend/security"
)

type message struct {
	ID                             int64
	Channel, Destination, Template string
	Payload                        []byte
	Attempts                       int
}
type notificationWorker struct {
	repo            repository.DBRepository
	cipher          *security.Cipher
	client          *http.Client
	endpoint, token string
}

func StartNotifications(ctx context.Context, repo repository.DBRepository, cipher *security.Cipher) {
	w := &notificationWorker{repo: repo, cipher: cipher, client: &http.Client{Timeout: 12 * time.Second}, endpoint: os.Getenv("MESSAGING_API_URL"), token: os.Getenv("MESSAGING_API_TOKEN")}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !w.hasProvider() {
				continue
			}
			w.scheduleReminders()
			w.process()
		}
	}
}

func (w *notificationWorker) hasProvider() bool {
	if w.endpoint != "" && w.token != "" {
		return true
	}
	return w.hasChannelProvider("whatsapp") || w.hasChannelProvider("sms") || w.hasChannelProvider("email")
}

func (w *notificationWorker) hasChannelProvider(channel string) bool {
	switch channel {
	case "whatsapp":
		return os.Getenv("WHATSAPP_PHONE_NUMBER_ID") != "" && os.Getenv("WHATSAPP_ACCESS_TOKEN") != "" && os.Getenv("WHATSAPP_TEMPLATE_NAME") != ""
	case "sms":
		return os.Getenv("TWILIO_ACCOUNT_SID") != "" && os.Getenv("TWILIO_AUTH_TOKEN") != "" && os.Getenv("TWILIO_FROM_NUMBER") != ""
	case "email":
		return os.Getenv("SMTP_HOST") != "" && os.Getenv("SMTP_FROM") != ""
	default:
		return false
	}
}

func (w *notificationWorker) scheduleReminders() {
	configured := strings.ToLower(strings.TrimSpace(os.Getenv("REMINDER_CHANNEL")))
	channels := []string{configured}
	if configured == "" {
		channels = []string{"whatsapp"}
	} else if configured == "all" {
		channels = []string{"whatsapp", "sms", "email"}
	} else if configured != "whatsapp" && configured != "sms" && configured != "email" {
		channels = []string{"whatsapp"}
	}

	for _, channel := range channels {
		if !w.hasChannelProvider(channel) && (w.endpoint == "" || w.token == "") {
			continue
		}
		destinationExpr := "p.phone"
		if channel == "email" {
			destinationExpr = "p.email"
		}
		for _, hours := range []int{48, 24} {
			keyPrefix := fmt.Sprintf("recordatorio_%dh_%s_", hours, channel)
			messageText := fmt.Sprintf("Recordatorio: tienes una cita odontológica en %d horas.", hours)
			query := fmt.Sprintf(`INSERT INTO notification_outbox(patient_id,appointment_id,channel,destination,template,payload,payload_enc,scheduled_at,idempotency_key) SELECT p.id,a.id,$1,%s,$2,'{}'::jsonb,$3,now(),$4||a.id::text FROM appointments a JOIN patients p ON p.id=a.patient_id WHERE p.consent_for_contact=TRUE AND a.status IN ('programado','confirmado') AND a.starts_at BETWEEN now()+(($5-60)::text||' seconds')::interval AND now()+($5::text||' seconds')::interval AND COALESCE(%s,'')<>'' ON CONFLICT(idempotency_key) DO NOTHING`, destinationExpr, destinationExpr)
			payload, _ := json.Marshal(map[string]string{"message": messageText})
			encrypted, err := w.cipher.Encrypt(payload)
			if err == nil {
				_, _ = w.repo.Exec(query, channel, fmt.Sprintf("recordatorio_%dh", hours), encrypted, keyPrefix, hours*3600)
			}
		}
	}
}

func (w *notificationWorker) process() {
	tx, err := w.repo.Begin()
	if err != nil {
		return
	}
	rows, err := tx.Query(`SELECT id,channel,destination,template,payload_enc,attempts FROM notification_outbox
		WHERE status='pendiente' AND next_attempt_at<=now()
		AND ($1 OR channel='whatsapp') AND ($2 OR channel='sms') AND ($3 OR channel='email')
		ORDER BY scheduled_at FOR UPDATE SKIP LOCKED LIMIT 30`,
		w.endpoint != "" && w.token != "" || w.hasChannelProvider("whatsapp"),
		w.endpoint != "" && w.token != "" || w.hasChannelProvider("sms"),
		w.endpoint != "" && w.token != "" || w.hasChannelProvider("email"))
	if err != nil {
		_ = tx.Rollback()
		return
	}
	batch := []message{}
	for rows.Next() {
		var m message
		if rows.Scan(&m.ID, &m.Channel, &m.Destination, &m.Template, &m.Payload, &m.Attempts) == nil {
			batch = append(batch, m)
		}
	}
	_ = rows.Close()
	for _, m := range batch {
		_, _ = tx.Exec(`UPDATE notification_outbox SET status='procesando',attempts=attempts+1,last_attempt_at=now() WHERE id=$1`, m.ID)
	}
	if tx.Commit() != nil {
		return
	}
	for _, m := range batch {
		plain, e := w.cipher.Decrypt(m.Payload)
		if e != nil {
			w.fail(m, "no se pudo descifrar el mensaje")
			continue
		}
		response, e := w.send(m, plain)
		if e != nil {
			w.fail(m, e.Error())
			continue
		}
		_, _ = w.repo.Exec(`UPDATE notification_outbox SET status='enviado',sent_at=now(),provider_response=$1 WHERE id=$2`, response, m.ID)
	}
}

func (w *notificationWorker) send(m message, payload []byte) (string, error) {
	var data struct {
		Message string `json:"message"`
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return "", fmt.Errorf("payload de mensaje inválido: %w", err)
	}
	if data.Message == "" {
		return "", fmt.Errorf("el mensaje está vacío")
	}
	if w.endpoint != "" && w.token != "" {
		body, err := json.Marshal(map[string]any{"channel": m.Channel, "to": m.Destination, "template": m.Template, "payload": json.RawMessage(payload)})
		if err != nil {
			return "", err
		}
		req, err := http.NewRequest(http.MethodPost, w.endpoint, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+w.token)
		resp, err := w.client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		result, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("adaptador HTTP %d: %s", resp.StatusCode, string(result))
		}
		return string(result), nil
	}
	switch m.Channel {
	case "whatsapp":
		return w.sendWhatsApp(m.Destination, data.Message)
	case "sms":
		return w.sendSMS(m.Destination, data.Message)
	case "email":
		return w.sendEmail(m.Destination, m.Template, data.Subject, data.Message)
	default:
		return "", fmt.Errorf("canal de mensajería no soportado: %s", m.Channel)
	}
}

func (w *notificationWorker) sendWhatsApp(destination, text string) (string, error) {
	version := strings.Trim(strings.TrimSpace(os.Getenv("WHATSAPP_API_VERSION")), "/")
	if version == "" {
		version = "v21.0"
	}
	baseURL := strings.TrimRight(os.Getenv("WHATSAPP_API_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "https://graph.facebook.com"
	}
	endpoint := fmt.Sprintf("%s/%s/%s/messages", baseURL, version, url.PathEscape(os.Getenv("WHATSAPP_PHONE_NUMBER_ID")))
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                destination,
		"type":              "template",
		"template": map[string]any{
			"name": os.Getenv("WHATSAPP_TEMPLATE_NAME"),
			"language": map[string]string{
				"code": envOr("WHATSAPP_TEMPLATE_LANGUAGE", "es"),
			},
			"components": []any{map[string]any{
				"type": "body",
				"parameters": []any{map[string]string{
					"type": "text",
					"text": text,
				}},
			}},
		},
	}
	return w.postJSON(endpoint, os.Getenv("WHATSAPP_ACCESS_TOKEN"), payload)
}

func (w *notificationWorker) sendSMS(destination, text string) (string, error) {
	account := os.Getenv("TWILIO_ACCOUNT_SID")
	endpoint := strings.TrimRight(os.Getenv("TWILIO_API_BASE_URL"), "/")
	if endpoint == "" {
		endpoint = "https://api.twilio.com"
	}
	form := url.Values{
		"To":   {destination},
		"From": {os.Getenv("TWILIO_FROM_NUMBER")},
		"Body": {text},
	}
	req, err := http.NewRequest(http.MethodPost, endpoint+"/2010-04-01/Accounts/"+url.PathEscape(account)+"/Messages.json", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(account, os.Getenv("TWILIO_AUTH_TOKEN"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return w.doRequest(req)
}

func (w *notificationWorker) sendEmail(destination, template, subject, text string) (string, error) {
	host := os.Getenv("SMTP_HOST")
	port := envOr("SMTP_PORT", "587")
	timeoutSeconds, err := strconv.Atoi(envOr("SMTP_TIMEOUT_SECONDS", "10"))
	if err != nil || timeoutSeconds < 1 || timeoutSeconds > 60 {
		return "", fmt.Errorf("SMTP_TIMEOUT_SECONDS debe estar entre 1 y 60")
	}
	address := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", address, time.Duration(timeoutSeconds)*time.Second)
	if err != nil {
		return "", fmt.Errorf("conectar al servidor SMTP: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Duration(timeoutSeconds) * time.Second))
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return "", fmt.Errorf("iniciar sesión SMTP: %w", err)
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return "", fmt.Errorf("el servidor SMTP no anuncia STARTTLS; se rechaza el envío sin cifrado")
	}
	if err = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
		return "", fmt.Errorf("negociar TLS con SMTP: %w", err)
	}
	user, password := os.Getenv("SMTP_USERNAME"), os.Getenv("SMTP_PASSWORD")
	if user != "" || password != "" {
		if user == "" || password == "" {
			return "", fmt.Errorf("SMTP_USERNAME y SMTP_PASSWORD deben configurarse juntos")
		}
		if err = client.Auth(smtp.PlainAuth("", user, password, host)); err != nil {
			return "", fmt.Errorf("autenticación SMTP: %w", err)
		}
	}
	from := os.Getenv("SMTP_FROM")
	if subject == "" {
		subject = "Recordatorio de cita odontológica"
	}
	if strings.ContainsAny(from+destination+subject, "\r\n") {
		return "", fmt.Errorf("cabecera de correo inválida")
	}
	fromAddress, err := mail.ParseAddress(from)
	if err != nil {
		return "", fmt.Errorf("SMTP_FROM no contiene una dirección válida: %w", err)
	}
	toAddress, err := mail.ParseAddress(destination)
	if err != nil {
		return "", fmt.Errorf("destino de correo no válido: %w", err)
	}
	if err = client.Mail(fromAddress.Address); err != nil {
		return "", err
	}
	if err = client.Rcpt(toAddress.Address); err != nil {
		return "", err
	}
	writer, err := client.Data()
	if err != nil {
		return "", err
	}
	content := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", fromAddress.String(), toAddress.String(), subject, text)
	if _, err = io.WriteString(writer, content); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	if err = client.Quit(); err != nil {
		return "", err
	}
	return "SMTP accepted: " + template, nil
}

func (w *notificationWorker) postJSON(endpoint, token string, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return w.doRequest(req)
}

func (w *notificationWorker) doRequest(req *http.Request) (string, error) {
	resp, err := w.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("proveedor HTTP %d: %s", resp.StatusCode, string(body))
	}
	return string(body), nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func (w *notificationWorker) fail(m message, reason string) {
	next := time.Now().Add(time.Duration(1<<min(m.Attempts, 6)) * time.Minute)
	status := "pendiente"
	if m.Attempts >= 4 {
		status = "fallido"
	}
	_, _ = w.repo.Exec(`UPDATE notification_outbox SET status=$1,provider_response=$2,next_attempt_at=$3 WHERE id=$4`, status, reason, next, m.ID)
}
