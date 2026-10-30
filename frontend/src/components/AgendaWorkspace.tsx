import { useEffect, useMemo, useState } from 'react'
import { request } from '../services/api'
import type { Appointment, Chair, Professional } from '../types'

type ViewMode = 'day' | 'week'
const statuses = ['programado', 'confirmado', 'en_espera', 'en_atencion', 'atendido', 'cancelado', 'no_asistio']
const statusNames: Record<string, string> = { programado: 'Programado', confirmado: 'Confirmado', en_espera: 'En espera', en_atencion: 'En atención', atendido: 'Atendido', cancelado: 'Cancelado', no_asistio: 'No asistió' }

function clinicDate(value: string) {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: 'America/Guayaquil', year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date(value))
  const part = (type: string) => parts.find(item => item.type === type)?.value ?? ''
  return `${part('year')}-${part('month')}-${part('day')}`
}

function addDays(date: string, amount: number) {
  const value = new Date(`${date}T12:00:00Z`)
  value.setUTCDate(value.getUTCDate() + amount)
  return value.toISOString().slice(0, 10)
}

function monday(date: string) {
  const value = new Date(`${date}T12:00:00Z`)
  const offset = (value.getUTCDay() + 6) % 7
  value.setUTCDate(value.getUTCDate() - offset)
  return value.toISOString().slice(0, 10)
}

function todayInClinic() {
  return clinicDate(new Date().toISOString())
}

function AppointmentCard({ appointment, onStatusChange, onPerform }: {
  appointment: Appointment
  onStatusChange: (id: number, status: string) => void
  onPerform: (appointment: Appointment) => void
}) {
  const time = new Intl.DateTimeFormat('es-EC', { timeZone: 'America/Guayaquil', hour: '2-digit', minute: '2-digit' })
  return <article className={`agenda-event status-${appointment.status}`}>
    <strong>{time.format(new Date(appointment.starts_at))}–{time.format(new Date(appointment.ends_at))}</strong>
    <span>{appointment.patient_name}</span>
    <small>{appointment.service_name || 'Consulta'} · {appointment.professional_name}</small>
    <small>{appointment.chair_name}</small>
    <select aria-label={`Estado de cita ${appointment.id}`} value={appointment.status} onChange={event => onStatusChange(appointment.id, event.target.value)}>
      {statuses.map(status => <option key={status} value={status}>{statusNames[status]}</option>)}
    </select>
    {appointment.status === 'en_atencion' && appointment.service_id !== null && <button className="text-button" onClick={() => onPerform(appointment)}>Registrar procedimiento</button>}
  </article>
}

export default function AgendaWorkspace({ appointments, professionals, chairs, onStatusChange, onPerform }: {
  appointments: Appointment[]
  professionals: Professional[]
  chairs: Chair[]
  onStatusChange: (id: number, status: string) => void
  onPerform: (appointment: Appointment) => void
}) {
  const [mode, setMode] = useState<ViewMode>('week')
  const [date, setDate] = useState(todayInClinic)
  const [chairId, setChairId] = useState('')
  const [professionalId, setProfessionalId] = useState('')
  const [query, setQuery] = useState('')
  const [rangeAppointments, setRangeAppointments] = useState(appointments)
  const [loadError, setLoadError] = useState('')
  const weekStart = monday(date)
  const days = useMemo(() => mode === 'day' ? [date] : Array.from({ length: 7 }, (_, index) => addDays(weekStart, index)), [date, mode, weekStart])
  const from = mode === 'day' ? date : weekStart
  const to = mode === 'day' ? date : addDays(weekStart, 6)
  useEffect(() => {
    let active = true
    void request<Appointment[]>(`/appointments?from=${from}&to=${to}`)
      .then(result => { if (active) { setRangeAppointments(result); setLoadError('') } })
      .catch(error => { if (active) setLoadError(error instanceof Error ? error.message : 'No se pudo cargar la agenda') })
    return () => { active = false }
  }, [appointments, from, to])
  const filtered = rangeAppointments.filter(item =>
    (!chairId || item.chair_id === Number(chairId)) &&
    (!professionalId || item.professional_id === Number(professionalId)) &&
    `${item.patient_name} ${item.service_name} ${item.professional_name}`.toLowerCase().includes(query.toLowerCase()) &&
    days.includes(clinicDate(item.starts_at)),
  )
  const byDay = (day: string) => filtered
    .filter(item => clinicDate(item.starts_at) === day)
    .sort((a, b) => a.starts_at.localeCompare(b.starts_at))
  const move = (amount: number) => setDate(value => addDays(value, mode === 'week' ? amount * 7 : amount))
  const label = (day: string) => new Intl.DateTimeFormat('es-EC', { timeZone: 'UTC', weekday: 'short', day: '2-digit', month: 'short' }).format(new Date(`${day}T12:00:00Z`))

  return <section className="panel agenda-workspace">
    <div className="agenda-toolbar">
      <div className="agenda-view-toggle">
        <button className={mode === 'day' ? 'selected' : ''} onClick={() => setMode('day')}>Día</button>
        <button className={mode === 'week' ? 'selected' : ''} onClick={() => setMode('week')}>Semana</button>
      </div>
      <div className="agenda-date-nav">
        <button className="secondary" onClick={() => move(-1)} aria-label="Periodo anterior">‹</button>
        <input aria-label="Fecha de agenda" type="date" value={date} onChange={event => setDate(event.target.value)} />
        <button className="secondary" onClick={() => move(1)} aria-label="Periodo siguiente">›</button>
        <button className="text-button" onClick={() => setDate(todayInClinic())}>Hoy</button>
      </div>
      <label className="search"><input placeholder="Buscar paciente o servicio" value={query} onChange={event => setQuery(event.target.value)} /></label>
      <select aria-label="Filtrar por sillón" value={chairId} onChange={event => setChairId(event.target.value)}>
        <option value="">Todos los sillones</option>
        {chairs.map(chair => <option key={chair.id} value={chair.id}>{chair.name}</option>)}
      </select>
      <select aria-label="Filtrar por profesional" value={professionalId} onChange={event => setProfessionalId(event.target.value)}>
        <option value="">Todos los profesionales</option>
        {professionals.map(professional => <option key={professional.id} value={professional.id}>{professional.full_name}</option>)}
      </select>
    </div>
    <div className={`agenda-calendar ${mode}`}>
      {days.map(day => <section className="agenda-day" key={day}>
        <header><strong>{label(day)}</strong><span>{byDay(day).length} citas</span></header>
        {byDay(day).length ? byDay(day).map(item => <AppointmentCard key={item.id} appointment={item} onStatusChange={onStatusChange} onPerform={onPerform} />) : <div className="agenda-empty">Sin citas</div>}
      </section>)}
      {filtered.length === 0 && <p className="agenda-no-results">No hay citas para este periodo y filtros.</p>}
    </div>
    {loadError&&<p className="agenda-load-error" role="alert">{loadError}</p>}
  </section>
}
