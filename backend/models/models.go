package models

import "time"

type Role string

const (
	RoleAdmin     Role = "admin"
	RoleReception Role = "recepcionista"
	RoleDentist   Role = "odontologo"
	RoleAssistant Role = "asistente"
	RoleAuditor   Role = "auditor"
)

type Patient struct {
	ID               int64      `json:"id"`
	DocumentID       string     `json:"document_id"`
	FirstName        string     `json:"first_name"`
	LastName         string     `json:"last_name"`
	BirthDate        *time.Time `json:"birth_date,omitempty"`
	Phone            string     `json:"phone"`
	Email            string     `json:"email"`
	EmergencyContact string     `json:"emergency_contact"`
	EmergencyPhone   string     `json:"emergency_phone"`
	GuardianName     string     `json:"guardian_name"`
	Insurance        string     `json:"insurance"`
	MedicalAlerts    string     `json:"medical_alerts"`
	Allergies        string     `json:"allergies"`
	MedicalHistory   string     `json:"medical_history"`
}

type ClinicalSignature struct {
	UserID    int64     `json:"user_id"`
	SignedAt  time.Time `json:"signed_at"`
	Signature string    `json:"signature"`
}
