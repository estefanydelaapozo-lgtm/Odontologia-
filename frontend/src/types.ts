export type User = { id: number; email: string; name: string; role: 'admin' | 'recepcionista' | 'odontologo' | 'asistente' | 'auditor' | 'paciente'; mfa?: boolean }
export type Patient = { id: number; document_id: string; first_name: string; last_name: string; birth_date?: string; phone: string; email: string; emergency_contact: string; emergency_phone: string; guardian_name: string; medical_alerts: string; medical_history: string; allergies: string; insurance: string; insurance_policy: string; consent_for_contact: boolean }
export type Professional = { id: number; full_name: string; specialty: string; active: boolean }
export type Chair = { id: number; name: string; location: string; active: boolean }
export type Service = { id: number; code: string; name: string; description: string; duration_minutes: number; price: number; active: boolean }
export type Appointment = { id: number; patient_id: number; patient_name: string; professional_id: number; professional_name: string; chair_id: number; chair_name: string; service_id: number | null; treatment_item_id: number | null; service_name: string; starts_at: string; ends_at: string; status: string; notes: string }
export type Invoice = { id: number; patient_id: number; patient_name: string; total: number; paid: number; balance: number; status: string; fiscal_status: string; fiscal_reference: string; created_at: string }
