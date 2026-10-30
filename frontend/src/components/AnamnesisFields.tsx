import { useMemo } from 'react'

const conditions = [
  { value: 'cardiovascular', label: 'Enfermedad cardiovascular' },
  { value: 'hypertension', label: 'Hipertensión arterial' },
  { value: 'diabetes', label: 'Diabetes' },
  { value: 'anticoagulants', label: 'Uso de anticoagulantes' },
  { value: 'coagulopathy', label: 'Coagulopatía o sangrado anormal' },
  { value: 'penicillin_allergy', label: 'Alergia a penicilina' },
  { value: 'latex_allergy', label: 'Alergia al látex' },
  { value: 'anesthetic_allergy', label: 'Alergia a anestésicos locales' },
  { value: 'asthma', label: 'Asma' },
  { value: 'renal_disease', label: 'Enfermedad renal' },
  { value: 'hepatic_disease', label: 'Enfermedad hepática' },
  { value: 'immunosuppression', label: 'Inmunosupresión' },
  { value: 'pregnancy', label: 'Embarazo' },
  { value: 'surgery', label: 'Antecedentes quirúrgicos relevantes' },
]

type Anamnesis = { version: 1; conditions: string[]; notes: string }

export function parseAnamnesis(value: string): Anamnesis {
  try {
    const parsed: unknown = JSON.parse(value)
    if (parsed && typeof parsed === 'object' && 'version' in parsed && parsed.version === 1) {
      const data = parsed as Partial<Anamnesis>
      return {
        version: 1,
        conditions: Array.isArray(data.conditions) ? data.conditions.filter((item): item is string => typeof item === 'string') : [],
        notes: typeof data.notes === 'string' ? data.notes : '',
      }
    }
  } catch {
    // Older records contain free-text history; retain it as notes when editing.
  }
  return { version: 1, conditions: [], notes: value }
}

export function anamnesisAlerts(value: string): string[] {
  const history = parseAnamnesis(value)
  return conditions.filter(item => history.conditions.includes(item.value)).map(item => item.label)
}

export default function AnamnesisFields({ value }: { value: string }) {
  const history = useMemo(() => parseAnamnesis(value), [value])
  return <>
    <fieldset className="anamnesis-fieldset">
      <legend>Antecedentes estructurados · selecciona los que apliquen</legend>
      <div className="anamnesis-grid">
        {conditions.map(item => <label key={item.value}>
          <input type="checkbox" name="anamnesis_conditions" value={item.value} defaultChecked={history.conditions.includes(item.value)} />
          {item.label}
        </label>)}
      </div>
    </fieldset>
    <label className="full-field">Detalle de enfermedades, medicamentos, alergias y cirugías<textarea name="anamnesis_notes" defaultValue={history.notes} placeholder="Incluye medicamentos, dosis, antecedentes quirúrgicos y otros datos relevantes."/></label>
  </>
}
