package workers

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSendWhatsAppUsesApprovedTemplateAndToken(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v21.0/123/messages" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("missing provider token")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"messages":[{"id":"test"}]}`)
	}))
	defer server.Close()

	t.Setenv("WHATSAPP_PHONE_NUMBER_ID", "123")
	t.Setenv("WHATSAPP_ACCESS_TOKEN", "access-token")
	t.Setenv("WHATSAPP_TEMPLATE_NAME", "appointment_reminder")
	t.Setenv("WHATSAPP_TEMPLATE_LANGUAGE", "es")
	t.Setenv("WHATSAPP_API_VERSION", "v21.0")
	t.Setenv("WHATSAPP_API_BASE_URL", server.URL)

	w := &notificationWorker{client: server.Client()}
	response, err := w.sendWhatsApp("+593991234567", "Tu cita es mañana")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response, `"id":"test"`) {
		t.Fatalf("unexpected provider response: %s", response)
	}
	template := got["template"].(map[string]any)
	if template["name"] != "appointment_reminder" {
		t.Fatalf("unexpected template: %#v", template)
	}
	if got["to"] != "+593991234567" {
		t.Fatalf("unexpected destination: %#v", got["to"])
	}
}

func TestSendSMSUsesTwilioFormAndBasicAuth(t *testing.T) {
	var form url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "AC123" || password != "secret" {
			t.Errorf("Twilio basic authentication was not set")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read form: %v", err)
		}
		form, err = url.ParseQuery(string(body))
		if err != nil {
			t.Errorf("parse form: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"sid":"SM123"}`)
	}))
	defer server.Close()

	t.Setenv("TWILIO_ACCOUNT_SID", "AC123")
	t.Setenv("TWILIO_AUTH_TOKEN", "secret")
	t.Setenv("TWILIO_FROM_NUMBER", "+15005550006")
	t.Setenv("TWILIO_API_BASE_URL", server.URL)

	w := &notificationWorker{client: server.Client()}
	response, err := w.sendSMS("+593991234567", "Recordatorio")
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("To") != "+593991234567" || form.Get("From") != "+15005550006" || form.Get("Body") != "Recordatorio" {
		t.Fatalf("unexpected Twilio form: %v", form)
	}
	if !strings.Contains(response, "SM123") {
		t.Fatalf("unexpected provider response: %s", response)
	}
}

func TestSendRejectsInvalidPayloadAndProviderErrors(t *testing.T) {
	w := &notificationWorker{client: http.DefaultClient}
	if _, err := w.send(message{Channel: "sms"}, []byte(`{}`)); err == nil {
		t.Fatal("expected empty message error")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	w.endpoint, w.token, w.client = server.URL, "token", server.Client()
	if _, err := w.send(message{Channel: "email"}, []byte(`{"message":"hello"}`)); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected provider status in error, got %v", err)
	}
}

func TestHasChannelProviderRequiresCredentials(t *testing.T) {
	t.Setenv("WHATSAPP_PHONE_NUMBER_ID", "123")
	t.Setenv("WHATSAPP_ACCESS_TOKEN", "")
	t.Setenv("WHATSAPP_TEMPLATE_NAME", "reminder")
	w := &notificationWorker{}
	if w.hasChannelProvider("whatsapp") {
		t.Fatal("WhatsApp must not be enabled without an access token")
	}
	t.Setenv("WHATSAPP_ACCESS_TOKEN", "token")
	if !w.hasChannelProvider("whatsapp") {
		t.Fatal("WhatsApp provider should be enabled with all required configuration")
	}
}

func TestSendEmailRejectsSMTPWithoutStartTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, "220 test ESMTP\r\n")
		_, _ = bufio.NewReader(conn).ReadString('\n')
		_, _ = io.WriteString(conn, "250-test\r\n250 SIZE 1024\r\n")
	}()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SMTP_HOST", host)
	t.Setenv("SMTP_PORT", port)
	t.Setenv("SMTP_FROM", "clinic@example.test")
	t.Setenv("SMTP_TIMEOUT_SECONDS", "2")
	w := &notificationWorker{}
	_, err = w.sendEmail("patient@example.test", "appointment", "", "Reminder")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected unencrypted SMTP to be rejected, got %v", err)
	}
}
