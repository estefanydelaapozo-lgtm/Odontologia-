package controllers

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"sigo-dental/backend/auth"
	"sigo-dental/backend/repository"
	"sigo-dental/backend/security"
)

var repo repository.DBRepository
var jwtSecret []byte
var dataCipher *security.Cipher

func Initialize(r repository.DBRepository, secret []byte, cipher *security.Cipher) {
	repo = r
	jwtSecret = secret
	dataCipher = cipher
}

func Health(c *fiber.Ctx) error {
	var one int
	if err := repo.QueryRow(`SELECT 1`).Scan(&one); err != nil {
		return c.Status(503).JSON(fiber.Map{"status": "degraded", "database": "unavailable"})
	}
	return c.JSON(fiber.Map{"status": "ok", "database": "ok", "service": "sigo-dental"})
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func fail(c *fiber.Ctx, code int, message string) error {
	return c.Status(code).JSON(fiber.Map{"message": message})
}
func userID(c *fiber.Ctx) int64 { v, _ := c.Locals("user_id").(int64); return v }
func audit(c *fiber.Ctx, action, entity, id string) {
	_, _ = repo.Exec(`INSERT INTO audit_log(user_id,action,entity,entity_id,ip_address) VALUES($1,$2,$3,$4,$5)`, userID(c), action, entity, id, c.IP())
}
func SeedAdmin() error {
	var n int
	if err := repo.QueryRow(`SELECT count(*) FROM users WHERE role='admin'`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	password := env("ADMIN_PASSWORD", "")
	if password == "" {
		return errors.New("configure ADMIN_PASSWORD en .env")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = repo.Exec(`INSERT INTO users(email,password_hash,full_name,role) VALUES($1,$2,'Administrador del sistema','admin') ON CONFLICT(email) DO NOTHING`, strings.ToLower(env("ADMIN_EMAIL", "admin@odontologia.local")), string(hash))
	return err
}

// SeedDemoUsers creates predictable accounts only for local demonstrations.
// Keep ENABLE_DEMO_USERS disabled in any shared or production environment.
func SeedDemoUsers() error {
	if strings.ToLower(env("ENABLE_DEMO_USERS", "false")) != "true" {
		return nil
	}
	users := []struct {
		email, name, role string
	}{
		{"admin@odontologia.local", "Administrador demo", "admin"},
		{"recepcion@odontologia.local", "Recepción demo", "recepcionista"},
		{"odontologo@odontologia.local", "Odontólogo demo", "odontologo"},
		{"asistente@odontologia.local", "Asistente dental demo", "asistente"},
		{"auditor@odontologia.local", "Auditoría demo", "auditor"},
		{"paciente@odontologia.local", "Paciente demo", "paciente"},
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	for _, u := range users {
		_, err = repo.Exec(`INSERT INTO users(email,password_hash,full_name,role,active)
			VALUES($1,$2,$3,$4,TRUE)
			ON CONFLICT(email) DO UPDATE SET password_hash=EXCLUDED.password_hash,
			full_name=EXCLUDED.full_name, role=EXCLUDED.role, active=TRUE`, u.email, string(hash), u.name, u.role)
		if err != nil {
			return err
		}
	}
	return nil
}

func Login(c *fiber.Ctx) error {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := c.BodyParser(&in); err != nil || in.Email == "" || in.Password == "" {
		return fail(c, 400, "Correo y contraseña son obligatorios")
	}
	var id int64
	var hash, role, name string
	var active, mfaEnabled bool
	var encryptedSecret []byte
	err := repo.QueryRow(`SELECT id,password_hash,role,full_name,active,mfa_enabled,coalesce(mfa_secret_ciphertext,''::bytea) FROM users WHERE lower(email)=lower($1)`, in.Email).Scan(&id, &hash, &role, &name, &active, &mfaEnabled, &encryptedSecret)
	if err != nil || !active || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		return fail(c, 401, "Credenciales inválidas")
	}
	if mfaEnabled {
		if strings.TrimSpace(in.Code) == "" {
			return c.JSON(fiber.Map{"mfa_required": true})
		}
		secret, decErr := dataCipher.Decrypt(encryptedSecret)
		if decErr != nil || !totp.Validate(in.Code, string(secret)) {
			return fail(c, 401, "Código MFA inválido")
		}
	}
	claims := auth.Claims{UserID: id, Email: strings.ToLower(in.Email), Role: role, Name: name, MFA: mfaEnabled, RegisteredClaims: jwt.RegisteredClaims{Subject: strconv.FormatInt(id, 10), ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)), IssuedAt: jwt.NewNumericDate(time.Now())}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
	if err != nil {
		return fail(c, 500, "No se pudo iniciar sesión")
	}
	_, _ = repo.Exec(`UPDATE users SET last_login_at=now() WHERE id=$1`, id)
	return c.JSON(fiber.Map{"token": token, "user": fiber.Map{"id": id, "email": claims.Email, "role": role, "name": name, "mfa": mfaEnabled}})
}

func SetupMFA(c *fiber.Ctx) error {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "SIGO Dental", AccountName: fmt.Sprintf("%s", c.Locals("email"))})
	if err != nil {
		return fail(c, 500, "No se pudo preparar MFA")
	}
	encrypted, err := dataCipher.Encrypt([]byte(key.Secret()))
	if err != nil {
		return fail(c, 500, "No se pudo proteger MFA")
	}
	if _, err = repo.Exec(`UPDATE users SET mfa_setup_secret_enc=$1 WHERE id=$2`, encrypted, userID(c)); err != nil {
		return fail(c, 500, "No se pudo guardar la configuración MFA")
	}
	img, err := key.Image(220, 220)
	if err != nil {
		return fail(c, 500, "No se pudo generar el QR de configuración")
	}
	var buffer bytes.Buffer
	if err = png.Encode(&buffer, img); err != nil {
		return fail(c, 500, "No se pudo codificar el QR")
	}
	return c.JSON(fiber.Map{"secret": key.Secret(), "otpauth_url": key.URL(), "qr_data_url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes()), "issuer": "SIGO Dental"})
}

func ConfirmMFA(c *fiber.Ctx) error {
	var input struct {
		Code string `json:"code"`
	}
	if c.BodyParser(&input) != nil || strings.TrimSpace(input.Code) == "" {
		return fail(c, 400, "Código de tu aplicación autenticadora obligatorio")
	}
	var encrypted []byte
	if err := repo.QueryRow(`SELECT coalesce(mfa_setup_secret_enc,''::bytea) FROM users WHERE id=$1`, userID(c)).Scan(&encrypted); err != nil || len(encrypted) == 0 {
		return fail(c, 400, "Inicia la configuración MFA primero")
	}
	secret, err := dataCipher.Decrypt(encrypted)
	if err != nil {
		return fail(c, 500, "No se pudo leer la configuración MFA")
	}
	if !totp.Validate(input.Code, string(secret)) {
		return fail(c, 401, "Código MFA incorrecto")
	}
	if _, err = repo.Exec(`UPDATE users SET mfa_enabled=TRUE,mfa_secret_ciphertext=mfa_setup_secret_enc,mfa_setup_secret_enc=NULL WHERE id=$1`, userID(c)); err != nil {
		return fail(c, 500, "No se pudo activar MFA")
	}
	var email, role, name string
	if err = repo.QueryRow(`SELECT email,role,full_name FROM users WHERE id=$1`, userID(c)).Scan(&email, &role, &name); err != nil {
		return fail(c, 500, "No se pudo completar MFA")
	}
	claims := auth.Claims{UserID: userID(c), Email: email, Role: role, Name: name, MFA: true, RegisteredClaims: jwt.RegisteredClaims{Subject: strconv.FormatInt(userID(c), 10), ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)), IssuedAt: jwt.NewNumericDate(time.Now())}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
	if err != nil {
		return fail(c, 500, "No se pudo renovar la sesión")
	}
	audit(c, "activar_mfa", "usuario", strconv.FormatInt(userID(c), 10))
	return c.JSON(fiber.Map{"token": token, "user": fiber.Map{"id": userID(c), "email": email, "role": role, "name": name, "mfa": true}})
}
func Me(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"id": userID(c), "email": c.Locals("email"), "name": c.Locals("name"), "role": c.Locals("role")})
}
func ListUsers(c *fiber.Ctx) error {
	rows, err := repo.Query(`SELECT id,email,full_name,role,active,created_at FROM users ORDER BY full_name`)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar usuarios")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id int64
		var email, name, role string
		var active bool
		var created time.Time
		if rows.Scan(&id, &email, &name, &role, &active, &created) == nil {
			out = append(out, fiber.Map{"id": id, "email": email, "full_name": name, "role": role, "active": active, "created_at": created})
		}
	}
	return c.JSON(out)
}
func CreateUser(c *fiber.Ctx) error {
	var x struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if c.BodyParser(&x) != nil || strings.TrimSpace(x.Email) == "" || strings.TrimSpace(x.FullName) == "" || len(x.Password) < 8 {
		return fail(c, 400, "Nombre, correo y contraseña de al menos 8 caracteres son obligatorios")
	}
	roles := map[string]bool{"admin": true, "recepcionista": true, "odontologo": true, "asistente": true, "auditor": true}
	if !roles[x.Role] {
		return fail(c, 400, "Rol no válido")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(x.Password), bcrypt.DefaultCost)
	if err != nil {
		return fail(c, 500, "No se pudo crear la cuenta")
	}
	var id int64
	err = repo.QueryRow(`INSERT INTO users(email,password_hash,full_name,role) VALUES(lower($1),$2,$3,$4) RETURNING id`, strings.TrimSpace(x.Email), string(hash), strings.TrimSpace(x.FullName), x.Role).Scan(&id)
	if err != nil {
		return fail(c, 409, "No se pudo crear la cuenta; verifica que el correo no esté registrado")
	}
	audit(c, "crear", "usuario", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id})
}
func parseID(c *fiber.Ctx) (int64, error) { return strconv.ParseInt(c.Params("id"), 10, 64) }

type patientInput struct {
	DocumentID        string `json:"document_id"`
	FirstName         string `json:"first_name"`
	LastName          string `json:"last_name"`
	BirthDate         string `json:"birth_date"`
	Phone             string `json:"phone"`
	Email             string `json:"email"`
	EmergencyContact  string `json:"emergency_contact"`
	EmergencyPhone    string `json:"emergency_phone"`
	GuardianName      string `json:"guardian_name"`
	Insurance         string `json:"insurance"`
	InsurancePolicy   string `json:"insurance_policy"`
	MedicalAlerts     string `json:"medical_alerts"`
	Allergies         string `json:"allergies"`
	MedicalHistory    string `json:"medical_history"`
	ConsentForContact bool   `json:"consent_for_contact"`
}

func ListPatients(c *fiber.Ctx) error {
	audit(c, "consultar", "pacientes", "listado")
	q := `SELECT id,coalesce(document_id,''),first_name,last_name,birth_date,coalesce(phone,''),coalesce(email,''),coalesce(emergency_contact,''),coalesce(emergency_phone,''),coalesce(guardian_name,''),coalesce(insurance,''),coalesce(insurance_policy,''),coalesce(medical_alerts_enc,''::bytea),coalesce(allergies_enc,''::bytea),coalesce(medical_history_enc,''::bytea),medical_alerts,allergies,medical_history,consent_for_contact,created_at FROM patients`
	args := []any{}
	if term := strings.TrimSpace(c.Query("q")); term != "" {
		q += ` WHERE first_name ILIKE $1 OR last_name ILIKE $1 OR coalesce(document_id,'') ILIKE $1`
		args = append(args, "%"+term+"%")
	}
	q += ` ORDER BY last_name,first_name LIMIT 250`
	rows, err := repo.Query(q, args...)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar pacientes")
	}
	defer rows.Close()
	type patient struct {
		ID                int64      `json:"id"`
		DocumentID        string     `json:"document_id"`
		FirstName         string     `json:"first_name"`
		LastName          string     `json:"last_name"`
		BirthDate         *time.Time `json:"birth_date"`
		Phone             string     `json:"phone"`
		Email             string     `json:"email"`
		EmergencyContact  string     `json:"emergency_contact"`
		EmergencyPhone    string     `json:"emergency_phone"`
		GuardianName      string     `json:"guardian_name"`
		Insurance         string     `json:"insurance"`
		InsurancePolicy   string     `json:"insurance_policy"`
		MedicalAlerts     string     `json:"medical_alerts"`
		Allergies         string     `json:"allergies"`
		MedicalHistory    string     `json:"medical_history"`
		ConsentForContact bool       `json:"consent_for_contact"`
		CreatedAt         time.Time  `json:"created_at"`
	}
	out := []patient{}
	for rows.Next() {
		var p patient
		var alertsEnc, allergiesEnc, historyEnc []byte
		var legacyAlerts, legacyAllergies, legacyHistory string
		if err = rows.Scan(&p.ID, &p.DocumentID, &p.FirstName, &p.LastName, &p.BirthDate, &p.Phone, &p.Email, &p.EmergencyContact, &p.EmergencyPhone, &p.GuardianName, &p.Insurance, &p.InsurancePolicy, &alertsEnc, &allergiesEnc, &historyEnc, &legacyAlerts, &legacyAllergies, &legacyHistory, &p.ConsentForContact, &p.CreatedAt); err != nil {
			return fail(c, 500, "Error leyendo pacientes")
		}
		if len(alertsEnc) > 0 {
			plain, e := dataCipher.Decrypt(alertsEnc)
			if e != nil {
				return fail(c, 500, "No se pudo descifrar información clínica")
			}
			p.MedicalAlerts = string(plain)
		} else {
			p.MedicalAlerts = legacyAlerts
		}
		if len(allergiesEnc) > 0 {
			plain, e := dataCipher.Decrypt(allergiesEnc)
			if e != nil {
				return fail(c, 500, "No se pudo descifrar información clínica")
			}
			p.Allergies = string(plain)
		} else {
			p.Allergies = legacyAllergies
		}
		if len(historyEnc) > 0 {
			plain, e := dataCipher.Decrypt(historyEnc)
			if e != nil {
				return fail(c, 500, "No se pudo descifrar información clínica")
			}
			p.MedicalHistory = string(plain)
		} else {
			p.MedicalHistory = legacyHistory
		}
		out = append(out, p)
	}
	return c.JSON(out)
}
func savePatient(c *fiber.Ctx, update bool) error {
	var p patientInput
	if err := c.BodyParser(&p); err != nil {
		return fail(c, 400, "Datos del paciente inválidos")
	}
	p.FirstName = strings.TrimSpace(p.FirstName)
	p.LastName = strings.TrimSpace(p.LastName)
	if p.FirstName == "" || p.LastName == "" {
		return fail(c, 400, "Nombre y apellido son obligatorios")
	}
	if strings.TrimSpace(p.Allergies) == "" || strings.TrimSpace(p.MedicalHistory) == "" {
		return fail(c, 400, "Completa la anamnesis médica y el campo de alergias (puedes indicar 'Ninguno')")
	}
	var birth any
	var birthDate *time.Time
	if p.BirthDate != "" {
		d, err := time.Parse("2006-01-02", p.BirthDate)
		if err != nil {
			return fail(c, 400, "Fecha de nacimiento inválida")
		}
		birth = d
		birthDate = &d
	}
	if birthDate != nil && birthDate.After(time.Now().AddDate(-18, 0, 0)) && strings.TrimSpace(p.GuardianName) == "" {
		return fail(c, 400, "Registra el tutor legal para pacientes menores de edad")
	}
	alertsEnc, err := dataCipher.Encrypt([]byte(p.MedicalAlerts))
	if err != nil {
		return fail(c, 500, "No se pudo proteger la información clínica")
	}
	allergiesEnc, err := dataCipher.Encrypt([]byte(p.Allergies))
	if err != nil {
		return fail(c, 500, "No se pudo proteger la información clínica")
	}
	historyEnc, err := dataCipher.Encrypt([]byte(p.MedicalHistory))
	if err != nil {
		return fail(c, 500, "No se pudo proteger la información clínica")
	}
	var id int64
	if update {
		id, err = parseID(c)
		if err != nil {
			return fail(c, 400, "ID inválido")
		}
		res, e := repo.Exec(`UPDATE patients SET document_id=NULLIF($1,''),first_name=$2,last_name=$3,birth_date=$4,phone=$5,email=$6,emergency_contact=$7,emergency_phone=$8,guardian_name=$9,insurance=$10,insurance_policy=$11,medical_alerts='',allergies='',medical_history='',medical_alerts_enc=$12,allergies_enc=$13,medical_history_enc=$14,consent_for_contact=$15,updated_at=now() WHERE id=$16`, p.DocumentID, p.FirstName, p.LastName, birth, p.Phone, p.Email, p.EmergencyContact, p.EmergencyPhone, p.GuardianName, p.Insurance, p.InsurancePolicy, alertsEnc, allergiesEnc, historyEnc, p.ConsentForContact, id)
		if e != nil {
			return fail(c, 409, "No se pudo actualizar (verifica que el documento no esté duplicado)")
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return fail(c, 404, "Paciente no encontrado")
		}
		audit(c, "actualizar", "paciente", strconv.FormatInt(id, 10))
		return c.JSON(fiber.Map{"id": id})
	}
	err = repo.QueryRow(`INSERT INTO patients(document_id,first_name,last_name,birth_date,phone,email,emergency_contact,emergency_phone,guardian_name,insurance,insurance_policy,medical_alerts,allergies,medical_history,medical_alerts_enc,allergies_enc,medical_history_enc,consent_for_contact) VALUES(NULLIF($1,''),$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'','','',$12,$13,$14,$15) RETURNING id`, p.DocumentID, p.FirstName, p.LastName, birth, p.Phone, p.Email, p.EmergencyContact, p.EmergencyPhone, p.GuardianName, p.Insurance, p.InsurancePolicy, alertsEnc, allergiesEnc, historyEnc, p.ConsentForContact).Scan(&id)
	if err != nil {
		return fail(c, 409, "No se pudo registrar el paciente; verifica el documento de identidad")
	}
	audit(c, "crear", "paciente", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id})
}
func CreatePatient(c *fiber.Ctx) error { return savePatient(c, false) }
func UpdatePatient(c *fiber.Ctx) error { return savePatient(c, true) }

func ListProfessionals(c *fiber.Ctx) error {
	rows, err := repo.Query(`SELECT id,full_name,specialty,coalesce(license_number,''),active FROM professionals ORDER BY full_name`)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar profesionales")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id int64
		var n, s, l string
		var a bool
		if rows.Scan(&id, &n, &s, &l, &a) == nil {
			out = append(out, fiber.Map{"id": id, "full_name": n, "specialty": s, "license_number": l, "active": a})
		}
	}
	return c.JSON(out)
}
func CreateProfessional(c *fiber.Ctx) error {
	var x struct {
		FullName  string `json:"full_name"`
		Specialty string `json:"specialty"`
		License   string `json:"license_number"`
	}
	if c.BodyParser(&x) != nil || strings.TrimSpace(x.FullName) == "" {
		return fail(c, 400, "Nombre del profesional obligatorio")
	}
	var id int64
	err := repo.QueryRow(`INSERT INTO professionals(full_name,specialty,license_number) VALUES($1,$2,$3) RETURNING id`, x.FullName, envOr(x.Specialty, "Odontología general"), x.License).Scan(&id)
	if err != nil {
		return fail(c, 409, "No se pudo guardar el profesional")
	}
	audit(c, "crear", "profesional", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id})
}
func ListChairs(c *fiber.Ctx) error {
	rows, err := repo.Query(`SELECT id,name,location,active FROM chairs ORDER BY name`)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar sillones")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id int64
		var n, l string
		var a bool
		if rows.Scan(&id, &n, &l, &a) == nil {
			out = append(out, fiber.Map{"id": id, "name": n, "location": l, "active": a})
		}
	}
	return c.JSON(out)
}
func CreateChair(c *fiber.Ctx) error {
	var x struct {
		Name     string `json:"name"`
		Location string `json:"location"`
	}
	if c.BodyParser(&x) != nil || strings.TrimSpace(x.Name) == "" {
		return fail(c, 400, "Nombre del sillón obligatorio")
	}
	var id int64
	err := repo.QueryRow(`INSERT INTO chairs(name,location) VALUES($1,$2) RETURNING id`, x.Name, x.Location).Scan(&id)
	if err != nil {
		return fail(c, 409, "Ese sillón ya existe o no pudo guardarse")
	}
	return c.Status(201).JSON(fiber.Map{"id": id})
}
func ListServices(c *fiber.Ctx) error {
	rows, err := repo.Query(`SELECT id,coalesce(code,''),name,description,duration_minutes,price,active FROM services ORDER BY name`)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar servicios")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id int64
		var code, n, d string
		var mins int
		var price float64
		var a bool
		if rows.Scan(&id, &code, &n, &d, &mins, &price, &a) == nil {
			out = append(out, fiber.Map{"id": id, "code": code, "name": n, "description": d, "duration_minutes": mins, "price": price, "active": a})
		}
	}
	return c.JSON(out)
}
func CreateService(c *fiber.Ctx) error {
	var x struct {
		Code        string  `json:"code"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Duration    int     `json:"duration_minutes"`
		Price       float64 `json:"price"`
	}
	if c.BodyParser(&x) != nil || strings.TrimSpace(x.Name) == "" || x.Price < 0 || x.Duration < 1 {
		return fail(c, 400, "Nombre, duración y precio válidos son obligatorios")
	}
	var id int64
	err := repo.QueryRow(`INSERT INTO services(code,name,description,duration_minutes,price) VALUES(NULLIF($1,''),$2,$3,$4,$5) RETURNING id`, x.Code, x.Name, x.Description, x.Duration, x.Price).Scan(&id)
	if err != nil {
		return fail(c, 409, "No se pudo guardar el servicio (verifica el código)")
	}
	return c.Status(201).JSON(fiber.Map{"id": id})
}
func envOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func ListAppointments(c *fiber.Ctx) error {
	location := clinicLocation()
	today := time.Now().In(location)
	from, err := time.ParseInLocation("2006-01-02", c.Query("from", today.AddDate(0, 0, -7).Format("2006-01-02")), location)
	if err != nil {
		return fail(c, 400, "Fecha inicial inválida; usa YYYY-MM-DD")
	}
	to, err := time.ParseInLocation("2006-01-02", c.Query("to", today.AddDate(0, 0, 30).Format("2006-01-02")), location)
	if err != nil || to.Before(from) || to.Sub(from) > 92*24*time.Hour {
		return fail(c, 400, "Rango de fechas inválido (máximo 92 días)")
	}
	rows, err := repo.Query(`SELECT a.id,a.patient_id,p.first_name||' '||p.last_name,a.professional_id,pr.full_name,a.chair_id,ch.name,a.service_id,a.treatment_item_id,coalesce(s.name,''),a.starts_at,a.ends_at,a.status,a.notes FROM appointments a JOIN patients p ON p.id=a.patient_id JOIN professionals pr ON pr.id=a.professional_id JOIN chairs ch ON ch.id=a.chair_id LEFT JOIN services s ON s.id=a.service_id WHERE a.starts_at >= $1 AND a.starts_at < $2 ORDER BY a.starts_at`, from, to.AddDate(0, 0, 1))
	if err != nil {
		return fail(c, 500, "No se pudieron consultar las citas")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id, patient, pro, chair int64
		var service sql.NullInt64
		var treatmentItem sql.NullInt64
		var pn, pr, cn, sn, status, notes string
		var start, end time.Time
		if rows.Scan(&id, &patient, &pn, &pro, &pr, &chair, &cn, &service, &treatmentItem, &sn, &start, &end, &status, &notes) != nil {
			continue
		}
		var serviceID, planItemID any
		if service.Valid {
			serviceID = service.Int64
		}
		if treatmentItem.Valid {
			planItemID = treatmentItem.Int64
		}
		out = append(out, fiber.Map{"id": id, "patient_id": patient, "patient_name": pn, "professional_id": pro, "professional_name": pr, "chair_id": chair, "chair_name": cn, "service_id": serviceID, "treatment_item_id": planItemID, "service_name": sn, "starts_at": start, "ends_at": end, "status": status, "notes": notes})
	}
	return c.JSON(out)
}
func CreateAppointment(c *fiber.Ctx) error {
	var x struct {
		PatientID       int64     `json:"patient_id"`
		ProfessionalID  int64     `json:"professional_id"`
		ChairID         int64     `json:"chair_id"`
		ServiceID       *int64    `json:"service_id"`
		TreatmentItemID *int64    `json:"treatment_item_id"`
		StartsAt        time.Time `json:"starts_at"`
		EndsAt          time.Time `json:"ends_at"`
		Notes           string    `json:"notes"`
	}
	if c.BodyParser(&x) != nil || x.PatientID == 0 || x.ProfessionalID == 0 || x.ChairID == 0 || !x.EndsAt.After(x.StartsAt) {
		return fail(c, 400, "Paciente, profesional, sillón y horario válido son obligatorios")
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo reservar la cita")
	}
	defer tx.Rollback()
	var available int
	if err = tx.QueryRow(`SELECT count(*) FROM patients p, professionals pr, chairs ch WHERE p.id=$1 AND pr.id=$2 AND pr.active=TRUE AND ch.id=$3 AND ch.active=TRUE`, x.PatientID, x.ProfessionalID, x.ChairID).Scan(&available); err != nil || available != 1 {
		return fail(c, 400, "Paciente, profesional o sillón no disponible")
	}
	var lockResult int
	if err = tx.QueryRow(`SELECT 1 FROM (SELECT pg_advisory_xact_lock(1,$1)) AS lock`, x.ChairID).Scan(&lockResult); err != nil {
		return fail(c, 500, "No se pudo bloquear el sillón")
	}
	if err = tx.QueryRow(`SELECT 1 FROM (SELECT pg_advisory_xact_lock(2,$1)) AS lock`, x.ProfessionalID).Scan(&lockResult); err != nil {
		return fail(c, 500, "No se pudo bloquear el profesional")
	}
	var conflicts int
	err = tx.QueryRow(`SELECT count(*) FROM appointments WHERE status NOT IN ('cancelado','no_asistio') AND starts_at < $2 AND ends_at > $1 AND (chair_id=$3 OR professional_id=$4)`, x.StartsAt, x.EndsAt, x.ChairID, x.ProfessionalID).Scan(&conflicts)
	if err != nil {
		return fail(c, 500, "No se pudo validar disponibilidad")
	}
	if conflicts > 0 {
		return fail(c, 409, "El sillón o el profesional ya tiene una cita en ese horario")
	}
	if x.TreatmentItemID != nil {
		var approvedPatient int64
		err = tx.QueryRow(`SELECT p.patient_id FROM treatment_plan_items i JOIN treatment_plans p ON p.id=i.plan_id WHERE i.id=$1 AND i.approved_quantity>0 AND i.status='aprobado'`, *x.TreatmentItemID).Scan(&approvedPatient)
		if err != nil || approvedPatient != x.PatientID {
			return fail(c, 409, "El procedimiento no está aprobado para este paciente")
		}
	}
	var id int64
	err = tx.QueryRow(`INSERT INTO appointments(patient_id,professional_id,chair_id,service_id,treatment_item_id,starts_at,ends_at,notes,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, x.PatientID, x.ProfessionalID, x.ChairID, x.ServiceID, x.TreatmentItemID, x.StartsAt, x.EndsAt, x.Notes, userID(c)).Scan(&id)
	if err != nil {
		return fail(c, 400, "No se pudo crear la cita; verifica los datos seleccionados")
	}
	if x.TreatmentItemID != nil {
		_, _ = tx.Exec(`UPDATE treatment_plan_items SET status='agendado' WHERE id=$1`, *x.TreatmentItemID)
	}
	if err = queueAppointmentConfirmation(tx, id, x.PatientID); err != nil {
		return fail(c, 500, "No se pudo encolar la confirmación de la cita")
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar la cita")
	}
	audit(c, "crear", "cita", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id})
}

func queueAppointmentConfirmation(tx *sql.Tx, appointmentID, patientID int64) error {
	var consent bool
	var phone, email, firstName, service string
	var startsAt time.Time
	err := tx.QueryRow(`SELECT p.consent_for_contact,coalesce(p.phone,''),coalesce(p.email,''),p.first_name,
		coalesce(s.name,'consulta odontológica'),a.starts_at
		FROM appointments a JOIN patients p ON p.id=a.patient_id
		LEFT JOIN services s ON s.id=a.service_id WHERE a.id=$1 AND p.id=$2`, appointmentID, patientID).
		Scan(&consent, &phone, &email, &firstName, &service, &startsAt)
	if err != nil || !consent {
		return err
	}
	configured := strings.ToLower(strings.TrimSpace(env("REMINDER_CHANNEL", "whatsapp")))
	channels := []string{configured}
	if configured == "all" {
		channels = []string{"whatsapp", "sms", "email"}
	} else if configured != "whatsapp" && configured != "sms" && configured != "email" {
		return fmt.Errorf("REMINDER_CHANNEL no válido")
	}
	local := startsAt.In(clinicLocation())
	message := fmt.Sprintf("Hola, %s. Tu cita odontológica está programada para el %s a las %s. Servicio: %s.", firstName, local.Format("02/01/2006"), local.Format("15:04"), service)
	payload, err := json.Marshal(map[string]string{"message": message})
	if err != nil {
		return err
	}
	encrypted, err := dataCipher.Encrypt(payload)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		destination := phone
		if channel == "email" {
			destination = email
		}
		if strings.TrimSpace(destination) == "" {
			continue
		}
		key := fmt.Sprintf("confirmacion_cita_%s_%d", channel, appointmentID)
		if _, err = tx.Exec(`INSERT INTO notification_outbox(patient_id,appointment_id,channel,destination,template,payload,payload_enc,scheduled_at,idempotency_key)
			VALUES($1,$2,$3,$4,'confirmacion_cita','{}'::jsonb,$5,now(),$6) ON CONFLICT(idempotency_key) DO NOTHING`, patientID, appointmentID, channel, destination, encrypted, key); err != nil {
			return err
		}
	}
	return nil
}

func UpdateAppointmentStatus(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fail(c, 400, "ID inválido")
	}
	var x struct {
		Status string `json:"status"`
	}
	if c.BodyParser(&x) != nil {
		return fail(c, 400, "Estado inválido")
	}
	allowed := map[string]bool{"programado": true, "confirmado": true, "en_espera": true, "en_atencion": true, "cancelado": true, "no_asistio": true}
	if !allowed[x.Status] {
		return fail(c, 400, "Estado de cita no válido")
	}
	res, err := repo.Exec(`UPDATE appointments SET status=$1 WHERE id=$2 AND status NOT IN ('atendido','cancelado','no_asistio')`, x.Status, id)
	if err != nil {
		return fail(c, 500, "No se pudo actualizar la cita")
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fail(c, 409, "Cita no encontrada o su estado ya es definitivo")
	}
	if x.Status == "cancelado" || x.Status == "no_asistio" {
		_, _ = repo.Exec(`UPDATE notification_outbox SET status='fallido',provider_response='cita cancelada' WHERE appointment_id=$1 AND status='pendiente'`, id)
	}
	audit(c, "cambiar_estado", "cita", strconv.FormatInt(id, 10))
	return c.JSON(fiber.Map{"id": id, "status": x.Status})
}

func ListInvoices(c *fiber.Ctx) error {
	rows, err := repo.Query(`SELECT i.id,i.patient_id,p.first_name||' '||p.last_name,i.appointment_id,i.subtotal,i.discount,i.total,i.status,i.created_at,coalesce((SELECT sum(amount) FROM payments WHERE invoice_id=i.id),0),i.fiscal_status,coalesce(i.fiscal_reference,'') FROM invoices i JOIN patients p ON p.id=i.patient_id ORDER BY i.created_at DESC LIMIT 250`)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar cobros")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id, pid int64
		var aid sql.NullInt64
		var name, status, fiscalStatus, fiscalReference string
		var subtotal, discount, total, paid float64
		var created time.Time
		if rows.Scan(&id, &pid, &name, &aid, &subtotal, &discount, &total, &status, &created, &paid, &fiscalStatus, &fiscalReference) == nil {
			out = append(out, fiber.Map{"id": id, "patient_id": pid, "patient_name": name, "appointment_id": aid, "subtotal": subtotal, "discount": discount, "total": total, "paid": paid, "balance": total - paid, "status": status, "fiscal_status": fiscalStatus, "fiscal_reference": fiscalReference, "created_at": created})
		}
	}
	return c.JSON(out)
}
func CreateInvoice(c *fiber.Ctx) error {
	var x struct {
		PatientID     int64   `json:"patient_id"`
		AppointmentID *int64  `json:"appointment_id"`
		Subtotal      float64 `json:"subtotal"`
		Discount      float64 `json:"discount"`
	}
	if c.BodyParser(&x) != nil || x.PatientID == 0 || x.Subtotal < 0 || x.Discount < 0 || x.Discount > x.Subtotal {
		return fail(c, 400, "Paciente y valores válidos son obligatorios")
	}
	total := x.Subtotal - x.Discount
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo crear el cobro")
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRow(`INSERT INTO invoices(patient_id,appointment_id,subtotal,discount,total,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, x.PatientID, x.AppointmentID, x.Subtotal, x.Discount, total, userID(c)).Scan(&id)
	if err != nil {
		return fail(c, 400, "No se pudo crear el cobro; verifica paciente/cita")
	}
	if err = queueFiscalInvoice(tx, id, x.PatientID, total); err != nil {
		return fail(c, 500, "No se pudo preparar la emisión fiscal")
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar el cobro")
	}
	audit(c, "crear", "cobro", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id, "total": total, "status": "pendiente"})
}
func AddPayment(c *fiber.Ctx) error {
	id, err := parseID(c)
	if err != nil {
		return fail(c, 400, "ID inválido")
	}
	var x struct {
		Method    string  `json:"method"`
		Amount    float64 `json:"amount"`
		Reference string  `json:"reference"`
	}
	if c.BodyParser(&x) != nil || x.Amount <= 0 {
		return fail(c, 400, "Método y monto válido son obligatorios")
	}
	tx, err := repo.Begin()
	if err != nil {
		return fail(c, 500, "No se pudo registrar el pago")
	}
	defer tx.Rollback()
	var total float64
	var status string
	if err = tx.QueryRow(`SELECT total,status FROM invoices WHERE id=$1 FOR UPDATE`, id).Scan(&total, &status); err != nil {
		return fail(c, 404, "Cobro no encontrado")
	}
	var paid float64
	_ = tx.QueryRow(`SELECT coalesce(sum(amount),0) FROM payments WHERE invoice_id=$1`, id).Scan(&paid)
	if status == "anulada" || paid+x.Amount > total+0.001 {
		return fail(c, 409, "El monto supera el saldo pendiente o el cobro está anulado")
	}
	var cashSession any
	if x.Method == "efectivo" {
		var sid int64
		if err = tx.QueryRow(`SELECT id FROM cash_sessions WHERE user_id=$1 AND status='abierta' FOR UPDATE`, userID(c)).Scan(&sid); err != nil {
			return fail(c, 409, "Abre una caja antes de registrar pagos en efectivo")
		}
		cashSession = sid
	}
	_, err = tx.Exec(`INSERT INTO payments(invoice_id,method,amount,reference,created_by,cash_session_id) VALUES($1,$2,$3,$4,$5,$6)`, id, x.Method, x.Amount, x.Reference, userID(c), cashSession)
	if err != nil {
		return fail(c, 400, "Método de pago inválido")
	}
	if paid+x.Amount >= total {
		_, err = tx.Exec(`UPDATE invoices SET status='pagada' WHERE id=$1`, id)
		if err != nil {
			return fail(c, 500, "No se pudo cerrar el cobro")
		}
	}
	if tx.Commit() != nil {
		return fail(c, 500, "No se pudo confirmar el pago")
	}
	audit(c, "registrar_pago", "cobro", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"invoice_id": id, "paid": paid + x.Amount, "balance": total - paid - x.Amount})
}
func Dashboard(c *fiber.Ctx) error {
	tz := env("CLINIC_TZ", "America/Guayaquil")
	var patients, today, upcoming, activeTreatments, chairsInUse, lowStock int
	var incomeToday float64
	_ = repo.QueryRow(`SELECT count(*) FROM patients`).Scan(&patients)
	_ = repo.QueryRow(`SELECT count(*) FROM appointments WHERE (starts_at AT TIME ZONE $1)::date=(now() AT TIME ZONE $1)::date AND status NOT IN ('cancelado','no_asistio')`, tz).Scan(&today)
	_ = repo.QueryRow(`SELECT count(*) FROM appointments WHERE starts_at>=now() AND starts_at<now()+interval '7 days' AND status NOT IN ('cancelado','no_asistio')`).Scan(&upcoming)
	var outstanding float64
	_ = repo.QueryRow(`SELECT coalesce(sum(i.total-(SELECT coalesce(sum(p.amount),0) FROM payments p WHERE p.invoice_id=i.id)),0) FROM invoices i WHERE i.status='pendiente'`).Scan(&outstanding)
	_ = repo.QueryRow(`SELECT coalesce(sum(p.amount),0) FROM payments p JOIN invoices i ON i.id=p.invoice_id WHERE i.status<>'anulada' AND (p.created_at AT TIME ZONE $1)::date=(now() AT TIME ZONE $1)::date`, tz).Scan(&incomeToday)
	_ = repo.QueryRow(`SELECT count(*) FROM treatment_plan_items WHERE status IN ('propuesto','aprobado','agendado')`).Scan(&activeTreatments)
	_ = repo.QueryRow(`SELECT count(DISTINCT chair_id) FROM appointments WHERE (starts_at AT TIME ZONE $1)::date=(now() AT TIME ZONE $1)::date AND status NOT IN ('cancelado','no_asistio')`, tz).Scan(&chairsInUse)
	_ = repo.QueryRow(`SELECT count(*) FROM (SELECT i.id FROM inventory_items i LEFT JOIN inventory_balances b ON b.item_id=i.id WHERE i.active GROUP BY i.id HAVING coalesce(sum(b.quantity),0) <= i.minimum_stock) x`).Scan(&lowStock)
	return c.JSON(fiber.Map{"patients": patients, "appointments_today": today, "appointments_next_7_days": upcoming, "outstanding_balance": outstanding,
		"income_today": incomeToday, "active_treatments": activeTreatments, "chairs_in_use_today": chairsInUse, "low_stock_items": lowStock})
}
