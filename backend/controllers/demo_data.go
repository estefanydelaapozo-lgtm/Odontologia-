package controllers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// clinicLocation devuelve la zona horaria de la clínica (CLINIC_TZ, por defecto Ecuador continental).
func clinicLocation() *time.Location {
	if loc, err := time.LoadLocation(env("CLINIC_TZ", "America/Guayaquil")); err == nil {
		return loc
	}
	return time.FixedZone("ECT", -5*3600)
}

// SeedDemoData carga una base de demostración (pacientes, profesionales, sillones, servicios, agenda de hoy,
// cobros, planes de tratamiento e inventario). Sigue el mismo patrón que SeedDemoUsers: solo se ejecuta con
// ENABLE_DEMO_DATA=true y es idempotente (si ya existe la agenda demo, no repite nada).
// Los datos clínicos sensibles de los pacientes se guardan cifrados (AES-256-GCM), igual que al crearlos desde la app.
// TODOS los datos son ficticios; no usar en producción con pacientes reales.
func SeedDemoData() error {
	if strings.ToLower(env("ENABLE_DEMO_DATA", "false")) != "true" {
		return nil
	}
	var yaCargado bool
	if err := repo.QueryRow(`SELECT EXISTS (SELECT 1 FROM appointments WHERE notes LIKE 'Demo:%')`).Scan(&yaCargado); err != nil {
		return err
	}
	if yaCargado {
		return nil
	}
	var adminID int64
	if err := repo.QueryRow(`SELECT id FROM users WHERE role='admin' ORDER BY id LIMIT 1`).Scan(&adminID); err != nil {
		return fmt.Errorf("se necesita un administrador antes de cargar datos demo: %w", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// ---- Profesionales (con su usuario odontólogo) ----
	profesionales := []struct{ key, email, nombre, especialidad, registro string }{
		{"valeria", "valeria.ruiz@odontologia.local", "Dra. Valeria Ruiz", "Odontología general", "DEMO-0001"},
		{"daniel", "daniel.paredes@odontologia.local", "Dr. Daniel Paredes", "Endodoncia y ortodoncia", "DEMO-0002"},
	}
	prof := map[string]int64{}
	for _, p := range profesionales {
		var uid int64
		if err := repo.QueryRow(`INSERT INTO users(email,password_hash,full_name,role,active) VALUES($1,$2,$3,'odontologo',TRUE)
			ON CONFLICT(email) DO UPDATE SET full_name=EXCLUDED.full_name, role='odontologo', active=TRUE RETURNING id`, p.email, string(hash), p.nombre).Scan(&uid); err != nil {
			return fmt.Errorf("demo profesional %s: %w", p.email, err)
		}
		var pid int64
		if err := repo.QueryRow(`INSERT INTO professionals(user_id,full_name,specialty,license_number) VALUES($1,$2,$3,$4)
			ON CONFLICT(user_id) DO UPDATE SET full_name=EXCLUDED.full_name, specialty=EXCLUDED.specialty RETURNING id`, uid, p.nombre, p.especialidad, p.registro).Scan(&pid); err != nil {
			return fmt.Errorf("demo profesional %s: %w", p.nombre, err)
		}
		prof[p.key] = pid
	}

	// ---- Sillones ----
	sillon := map[string]int64{}
	for _, s := range []struct{ name, loc string }{{"Sillón 1", "Consultorio 1"}, {"Sillón 2", "Consultorio 2"}} {
		var id int64
		if err := repo.QueryRow(`INSERT INTO chairs(name,location) VALUES($1,$2) ON CONFLICT(name) DO UPDATE SET location=EXCLUDED.location RETURNING id`, s.name, s.loc).Scan(&id); err != nil {
			return err
		}
		sillon[s.name] = id
	}

	// ---- Servicios ----
	servicios := []struct {
		code, name string
		minutes    int
		price      float64
	}{
		{"VAL-01", "Valoración", 30, 20}, {"PRO-01", "Control y profilaxis", 45, 35}, {"END-01", "Endodoncia", 60, 120},
		{"ORT-01", "Control de ortodoncia", 30, 45}, {"RES-01", "Resina compuesta", 45, 40}, {"EXO-01", "Exodoncia simple", 40, 30},
		{"COR-01", "Corona de porcelana", 60, 250}, {"BLA-01", "Blanqueamiento dental", 60, 150},
	}
	svc := map[string]int64{}
	svcPrice := map[string]float64{}
	for _, s := range servicios {
		var id int64
		if err := repo.QueryRow(`INSERT INTO services(code,name,duration_minutes,price) VALUES($1,$2,$3,$4)
			ON CONFLICT(code) DO UPDATE SET name=EXCLUDED.name, duration_minutes=EXCLUDED.duration_minutes, price=EXCLUDED.price RETURNING id`, s.code, s.name, s.minutes, s.price).Scan(&id); err != nil {
			return err
		}
		svc[s.code], svcPrice[s.code] = id, s.price
	}

	// ---- Pacientes (datos clínicos cifrados) ----
	pacientes := []struct {
		key, doc, nombres, apellidos, nacimiento, tel, email, emergencia, telEmerg, tutor, seguro, alertas, alergias, historia string
	}{
		{"jose", "0912345671", "José Miguel", "Cedeño", "1970-03-12", "0991234501", "jose.cedeno@example.com", "Marta Cedeño", "0987654301", "", "", "Paciente anticoagulado (warfarina)", "Ninguna conocida", "Hipertensión arterial controlada. Sin cirugías previas."},
		{"sofia", "0912345672", "Sofía Mite", "González", "2014-06-20", "0991234502", "", "Rosa González", "0987654302", "Rosa González", "", "Alergia a penicilina", "Penicilina", "Sin antecedentes relevantes. Menor de edad: requiere tutor."},
		{"maria", "0912345673", "María Fernanda", "López", "1992-02-14", "0991234503", "mafe.lopez@example.com", "Pedro López", "0987654303", "", "Seguro Salud S.A.", "", "Ninguna", "Sin antecedentes médicos relevantes."},
		{"carlos", "0912345674", "Carlos Andrés", "Vera", "1984-08-05", "0991234504", "carlos.vera@example.com", "Lucía Vera", "0987654304", "", "", "Diabetes tipo 2", "Látex", "Diabetes tipo 2 en tratamiento. Control periódico de glucosa."},
	}
	pac := map[string]int64{}
	for _, p := range pacientes {
		alertas, err := dataCipher.Encrypt([]byte(p.alertas))
		if err != nil {
			return err
		}
		alergias, err := dataCipher.Encrypt([]byte(p.alergias))
		if err != nil {
			return err
		}
		historia, err := dataCipher.Encrypt([]byte(p.historia))
		if err != nil {
			return err
		}
		nac, err := time.Parse("2006-01-02", p.nacimiento)
		if err != nil {
			return err
		}
		var id int64
		if err := repo.QueryRow(`INSERT INTO patients(document_id,first_name,last_name,birth_date,phone,email,emergency_contact,emergency_phone,guardian_name,insurance,
			medical_alerts_enc,allergies_enc,medical_history_enc,consent_for_contact)
			VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,NULLIF($9,''),NULLIF($10,''),$11,$12,$13,TRUE)
			ON CONFLICT(document_id) DO UPDATE SET first_name=EXCLUDED.first_name RETURNING id`,
			p.doc, p.nombres, p.apellidos, nac, p.tel, p.email, p.emergencia, p.telEmerg, p.tutor, p.seguro, alertas, alergias, historia).Scan(&id); err != nil {
			return fmt.Errorf("demo paciente %s: %w", p.doc, err)
		}
		pac[p.key] = id
	}

	// ---- Inventario con receta de consumo ----
	insumos := []struct {
		sku, name, unit string
		min, qty        float64
	}{
		{"INS-RES", "Resina compuesta A2", "g", 10, 60}, {"INS-ANE", "Anestesia lidocaína 2% (cartucho)", "unidad", 20, 120},
		{"INS-GUA", "Guantes de nitrilo (par)", "par", 50, 300}, {"INS-ADH", "Adhesivo dental", "ml", 5, 25},
		{"INS-LIM", "Limas endodónticas (juego)", "juego", 4, 3}, {"INS-GUT", "Gutapercha (cono)", "unidad", 40, 200},
		{"INS-PAS", "Pasta profiláctica", "g", 10, 18},
	}
	ins := map[string]int64{}
	for _, i := range insumos {
		var id int64
		if err := repo.QueryRow(`INSERT INTO inventory_items(sku,name,unit,minimum_stock) VALUES($1,$2,$3,$4)
			ON CONFLICT(sku) DO UPDATE SET name=EXCLUDED.name RETURNING id`, i.sku, i.name, i.unit, i.min).Scan(&id); err != nil {
			return err
		}
		ins[i.sku] = id
		if _, err := repo.Exec(`INSERT INTO inventory_balances(item_id,location,quantity)
			SELECT $1::bigint,'Bodega principal',$2::numeric WHERE NOT EXISTS (SELECT 1 FROM inventory_balances WHERE item_id=$1 AND chair_id IS NULL AND location='Bodega principal')`, id, i.qty); err != nil {
			return err
		}
		if _, err := repo.Exec(`INSERT INTO inventory_movements(item_id,movement_type,quantity,reference_type,reference_id,recorded_by) VALUES($1,'entrada',$2,'saldo_inicial',0,$3)`, id, i.qty, adminID); err != nil {
			return err
		}
	}
	recetas := []struct {
		servicio, insumo string
		cantidad         float64
	}{
		{"PRO-01", "INS-PAS", 2}, {"PRO-01", "INS-GUA", 1},
		{"END-01", "INS-ANE", 2}, {"END-01", "INS-LIM", 1}, {"END-01", "INS-GUT", 3}, {"END-01", "INS-GUA", 1},
		{"RES-01", "INS-RES", 2}, {"RES-01", "INS-ANE", 1}, {"RES-01", "INS-ADH", 0.5}, {"RES-01", "INS-GUA", 1},
		{"EXO-01", "INS-ANE", 2}, {"EXO-01", "INS-GUA", 1},
	}
	for _, r := range recetas {
		if _, err := repo.Exec(`INSERT INTO service_materials(service_id,item_id,quantity) VALUES($1,$2,$3) ON CONFLICT(service_id,item_id) DO NOTHING`, svc[r.servicio], ins[r.insumo], r.cantidad); err != nil {
			return err
		}
	}

	// ---- Planes de tratamiento (12 ítems en seguimiento) ----
	type itemPlan struct {
		servicio, fase, estado string
		sesion                 int
		lab                    float64
	}
	planes := []struct {
		paciente, titulo, estado string
		items                    []itemPlan
	}{
		{"jose", "Plan integral de rehabilitación", "presentado", []itemPlan{{"PRO-01", "Fase 1", "propuesto", 1, 0}, {"RES-01", "Fase 1", "propuesto", 2, 0}, {"COR-01", "Fase 2", "propuesto", 1, 60}}},
		{"sofia", "Ortodoncia — fase inicial", "aprobado", []itemPlan{{"ORT-01", "Fase 1", "aprobado", 1, 0}, {"ORT-01", "Fase 1", "aprobado", 2, 0}, {"ORT-01", "Fase 1", "aprobado", 3, 0}}},
		{"maria", "Mantenimiento y estética", "aprobado_parcial", []itemPlan{{"PRO-01", "Fase 1", "aprobado", 1, 0}, {"BLA-01", "Fase 2", "propuesto", 1, 0}, {"RES-01", "Fase 2", "propuesto", 2, 0}}},
		{"carlos", "Tratamiento endodóntico y corona", "aprobado_parcial", []itemPlan{{"END-01", "Fase 1", "aprobado", 1, 0}, {"COR-01", "Fase 2", "aprobado", 1, 60}, {"RES-01", "Fase 2", "propuesto", 2, 0}}},
	}
	carlosEndoItem := int64(0)
	for _, pl := range planes {
		var planID int64
		if err := repo.QueryRow(`INSERT INTO treatment_plans(patient_id,title,status,created_by,approved_at) VALUES($1,$2,$3,$4,CASE WHEN $3 LIKE 'aprobado%' THEN now() ELSE NULL END) RETURNING id`,
			pac[pl.paciente], pl.titulo, pl.estado, adminID).Scan(&planID); err != nil {
			return err
		}
		for _, it := range pl.items {
			aprobada := 0
			if it.estado == "aprobado" {
				aprobada = 1
			}
			var itemID int64
			if err := repo.QueryRow(`INSERT INTO treatment_plan_items(plan_id,service_id,phase_name,session_number,professional_fee,laboratory_cost,quantity,approved_quantity,status)
				VALUES($1,$2,$3,$4,$5,$6,1,$7,$8) RETURNING id`, planID, svc[it.servicio], it.fase, it.sesion, svcPrice[it.servicio], it.lab, aprobada, it.estado).Scan(&itemID); err != nil {
				return err
			}
			if pl.paciente == "carlos" && it.servicio == "END-01" {
				carlosEndoItem = itemID
			}
		}
	}

	// ---- Agenda de hoy ----
	loc := clinicLocation()
	ahora := time.Now().In(loc)
	hoy := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 0, 0, 0, 0, loc)
	citas := []struct {
		paciente, profesional, silla, servicio, estado string
		inicio                                         time.Duration
		minutos                                        int
	}{
		{"maria", "valeria", "Sillón 1", "PRO-01", "confirmado", 9 * time.Hour, 45},
		{"carlos", "daniel", "Sillón 2", "END-01", "en_atencion", 9*time.Hour + 30*time.Minute, 60},
		{"sofia", "valeria", "Sillón 1", "ORT-01", "confirmado", 10 * time.Hour, 30},
		{"jose", "daniel", "Sillón 2", "VAL-01", "programado", 11 * time.Hour, 30},
	}
	citaPaciente := map[string]int64{}
	for _, ct := range citas {
		ini := hoy.Add(ct.inicio)
		fin := ini.Add(time.Duration(ct.minutos) * time.Minute)
		var id int64
		if err := repo.QueryRow(`INSERT INTO appointments(patient_id,professional_id,chair_id,service_id,starts_at,ends_at,status,notes,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,'Demo: cita de ejemplo',$8) RETURNING id`,
			pac[ct.paciente], prof[ct.profesional], sillon[ct.silla], svc[ct.servicio], ini, fin, ct.estado, adminID).Scan(&id); err != nil {
			return fmt.Errorf("demo cita %s: %w", ct.paciente, err)
		}
		citaPaciente[ct.paciente] = id
	}
	if carlosEndoItem != 0 {
		if _, err := repo.Exec(`UPDATE treatment_plan_items SET status='agendado' WHERE id=$1`, carlosEndoItem); err != nil {
			return err
		}
		if _, err := repo.Exec(`UPDATE appointments SET treatment_item_id=$1 WHERE id=$2`, carlosEndoItem, citaPaciente["carlos"]); err != nil {
			return err
		}
	}

	// ---- Cobros de hoy: ingresos por $165,00 (efectivo, tarjeta y transferencia) ----
	cobros := []struct {
		paciente string
		subtotal float64
		pagos    []struct {
			metodo string
			monto  float64
			ref    string
		}
	}{
		{"maria", 35, []struct {
			metodo string
			monto  float64
			ref    string
		}{{"efectivo", 35, ""}}},
		{"carlos", 130, []struct {
			metodo string
			monto  float64
			ref    string
		}{{"tarjeta", 100, "DEMO-POS-0001"}, {"transferencia", 30, "DEMO-TRF-0001"}}},
	}
	for _, cb := range cobros {
		var invID int64
		if err := repo.QueryRow(`INSERT INTO invoices(patient_id,appointment_id,subtotal,discount,total,status,created_by) VALUES($1,$2,$3,0,$3,'pagada',$4) RETURNING id`,
			pac[cb.paciente], citaPaciente[cb.paciente], cb.subtotal, adminID).Scan(&invID); err != nil {
			return err
		}
		for _, pg := range cb.pagos {
			if _, err := repo.Exec(`INSERT INTO payments(invoice_id,method,amount,reference,created_by) VALUES($1,$2,$3,$4,$5)`, invID, pg.metodo, pg.monto, pg.ref, adminID); err != nil {
				return err
			}
		}
	}

	// ---- Reglas de honorarios de ejemplo ----
	if _, err := repo.Exec(`INSERT INTO professional_fee_rules(professional_id,service_id,rule_type,rate) VALUES($1,NULL,'porcentaje',40),($2,NULL,'porcentaje_neto',50)`, prof["valeria"], prof["daniel"]); err != nil {
		return err
	}
	log.Println("demo: pacientes, agenda, cobros, planes e inventario de demostración cargados")
	return nil
}
