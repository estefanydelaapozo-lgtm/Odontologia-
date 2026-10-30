import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { downloadFile, post, put, request } from '../services/api'
import type { User } from '../types'

// Pantallas añadidas: Reportes (módulo 15), Auditoría (17), Usuarios y roles (16) y Expediente unificado (4).
// Reutilizan las clases CSS existentes (panel, table-wrap, form-grid, primary, secondary, badge...).

type Row = Record<string, unknown>
type ReportData = { kind: string; title: string; columns: string[]; rows: Row[] }

const isoDate = /^\d{4}-\d{2}-\d{2}T/
const fmt = (v: unknown): string => {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'string' && isoDate.test(v)) return new Date(v).toLocaleString('es-EC')
  if (typeof v === 'boolean') return v ? 'Sí' : 'No'
  return String(v)
}
const label = (c: string) => c.replaceAll('_', ' ').toUpperCase()
const today = () => new Date().toISOString().slice(0, 10)
const monthAgo = () => new Date(Date.now() - 30 * 86400000).toISOString().slice(0, 10)

function DataTable({ columns, rows, empty }: { columns: string[]; rows: Row[]; empty: string }) {
  return <div className="table-wrap"><table><thead><tr>{columns.map(c => <th key={c}>{label(c)}</th>)}</tr></thead><tbody>
    {rows.map((r, i) => <tr key={i}>{columns.map(c => <td key={c}>{fmt(r[c])}</td>)}</tr>)}
    {rows.length === 0 && <tr><td colSpan={columns.length}><div className="empty-inline">{empty}</div></td></tr>}
  </tbody></table></div>
}

function saveBlob(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a'); a.href = url; a.download = name; a.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}

const reportKinds: { id: string; name: string; roles: User['role'][] }[] = [
  { id: 'patients', name: 'Pacientes', roles: ['admin', 'recepcionista', 'auditor'] },
  { id: 'appointments', name: 'Citas', roles: ['admin', 'recepcionista', 'auditor'] },
  { id: 'income', name: 'Ingresos', roles: ['admin', 'auditor'] },
  { id: 'treatments', name: 'Tratamientos', roles: ['admin', 'auditor'] },
  { id: 'inventory', name: 'Inventario', roles: ['admin', 'auditor', 'asistente'] },
]

export function ReportsWorkspace({ user }: { user: User }) {
  const kinds = reportKinds.filter(k => k.roles.includes(user.role))
  const [kind, setKind] = useState(kinds[0]?.id ?? 'patients')
  const [desde, setDesde] = useState(monthAgo()), [hasta, setHasta] = useState(today())
  const [data, setData] = useState<ReportData | null>(null), [error, setError] = useState(''), [busy, setBusy] = useState(false)
  const query = `desde=${desde}&hasta=${hasta}`
  const load = useCallback(async () => {
    setBusy(true); setError('')
    try { setData(await request<ReportData>(`/reports/${kind}?desde=${desde}&hasta=${hasta}`)) }
    catch (e) { setData(null); setError(e instanceof Error ? e.message : 'No se pudo generar el reporte') }
    finally { setBusy(false) }
  }, [kind, desde, hasta])
  useEffect(() => { void load() }, [load])
  const exportCsv = async () => {
    try { saveBlob(await downloadFile(`/reports/${kind}?format=csv&${query}`), `reporte-${kind}-${today()}.csv`) }
    catch (e) { setError(e instanceof Error ? e.message : 'No se pudo exportar') }
  }
  return <section className="panel">
    <div className="panel-head"><div><h2>Reportes</h2><p>Pacientes, citas, ingresos, tratamientos e inventario. Cada consulta y exportación queda registrada en auditoría.</p></div></div>
    <div className="form-grid">
      <label>Reporte<select value={kind} onChange={e => setKind(e.target.value)}>{kinds.map(k => <option key={k.id} value={k.id}>{k.name}</option>)}</select></label>
      {kind !== 'inventory' && <><label>Desde<input type="date" value={desde} max={hasta} onChange={e => setDesde(e.target.value)} /></label>
        <label>Hasta<input type="date" value={hasta} min={desde} onChange={e => setHasta(e.target.value)} /></label></>}
      <div className="modal-actions"><button className="secondary" onClick={() => void load()} disabled={busy}>{busy ? 'Generando…' : 'Actualizar'}</button>
        <button className="primary" onClick={() => void exportCsv()} disabled={!data}>Exportar CSV</button></div>
    </div>
    {error && <div className="alert-error">{error}</div>}
    {data && <><p className="muted">{data.title} · {data.rows.length} registros</p><DataTable columns={data.columns} rows={data.rows} empty="No hay datos en el periodo seleccionado." /></>}
  </section>
}

export function AuditWorkspace() {
  const [rows, setRows] = useState<Row[]>([]), [error, setError] = useState('')
  const [f, setF] = useState({ usuario: '', accion: '', entidad: '', desde: monthAgo(), hasta: today() })
  const load = useCallback(async () => {
    setError('')
    const p = new URLSearchParams({ desde: f.desde, hasta: f.hasta, usuario: f.usuario, accion: f.accion, entidad: f.entidad })
    try { setRows(await request<Row[]>(`/audit-log?${p.toString()}`)) } catch (e) { setError(e instanceof Error ? e.message : 'No se pudo consultar la auditoría') }
  }, [f])
  useEffect(() => { void load() }, [load])
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF(v => ({ ...v, [k]: e.target.value }))
  return <section className="panel">
    <div className="panel-head"><div><h2>Auditoría de accesos y cambios</h2><p>Registro inmutable: quién consultó, modificó o exportó qué, con fecha, hora e IP.</p></div></div>
    <div className="form-grid">
      <label>Usuario<input value={f.usuario} onChange={set('usuario')} placeholder="Nombre o correo" /></label>
      <label>Acción<input value={f.accion} onChange={set('accion')} placeholder="consultar, exportar…" /></label>
      <label>Entidad<input value={f.entidad} onChange={set('entidad')} placeholder="pacientes, expediente…" /></label>
      <label>Desde<input type="date" value={f.desde} onChange={set('desde')} /></label>
      <label>Hasta<input type="date" value={f.hasta} onChange={set('hasta')} /></label>
    </div>
    {error && <div className="alert-error">{error}</div>}
    <DataTable columns={['created_at', 'usuario', 'correo', 'accion', 'entidad', 'referencia', 'ip']} rows={rows} empty="No hay eventos para los filtros elegidos." />
  </section>
}

type UserRow = { id: number; email: string; full_name: string; role: User['role']; active: boolean; created_at: string }
const roles: { id: User['role']; name: string }[] = [{ id: 'admin', name: 'Administrador TI' }, { id: 'recepcionista', name: 'Recepcionista / Cajero' },
  { id: 'odontologo', name: 'Odontólogo/a' }, { id: 'asistente', name: 'Asistente dental' }, { id: 'auditor', name: 'Auditor / Contador' }]

export function UsersWorkspace({ currentUserId }: { currentUserId: number }) {
  const [users, setUsers] = useState<UserRow[]>([]), [error, setError] = useState(''), [notice, setNotice] = useState('')
  const load = useCallback(async () => { try { setUsers(await request<UserRow[]>('/users')) } catch (e) { setError(e instanceof Error ? e.message : 'No se pudieron cargar usuarios') } }, [])
  useEffect(() => { void load() }, [load])
  const run = async (action: () => Promise<unknown>, ok: string) => {
    setError(''); setNotice('')
    try { await action(); setNotice(ok); await load() } catch (e) { setError(e instanceof Error ? e.message : 'No se pudo completar la acción') }
  }
  const save = (u: UserRow, patch: Partial<UserRow>) => run(() => put(`/users/${u.id}`, { full_name: u.full_name, role: u.role, active: u.active, ...patch }), 'Usuario actualizado')
  const create = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault(); const form = e.currentTarget; const d = Object.fromEntries(new FormData(form).entries())
    await run(() => post('/users', { full_name: d.full_name, email: d.email, role: d.role, password: d.password }), 'Usuario creado'); form.reset()
  }
  return <section className="panel">
    <div className="panel-head"><div><h2>Usuarios y roles</h2><p>Crea cuentas, cambia roles, desactiva accesos y restablece contraseña o MFA.</p></div></div>
    {error && <div className="alert-error">{error}</div>}{notice && <div className="clinical-notice">{notice}</div>}
    <form className="form-grid" onSubmit={e => void create(e)}>
      <label>Nombre completo<input name="full_name" required /></label><label>Correo<input name="email" type="email" required /></label>
      <label>Rol<select name="role" defaultValue="recepcionista">{roles.map(r => <option key={r.id} value={r.id}>{r.name}</option>)}</select></label>
      <label>Contraseña inicial (mín. 8)<input name="password" type="password" minLength={8} required /></label>
      <div className="modal-actions"><button className="primary">Crear usuario</button></div>
    </form>
    <div className="table-wrap"><table><thead><tr><th>NOMBRE</th><th>CORREO</th><th>ROL</th><th>ESTADO</th><th>ACCIONES</th></tr></thead><tbody>
      {users.map(u => <tr key={u.id}><td>{u.full_name}</td><td>{u.email}</td>
        <td><select value={u.role} disabled={u.id === currentUserId} onChange={e => void save(u, { role: e.target.value as User['role'] })}>{roles.map(r => <option key={r.id} value={r.id}>{r.name}</option>)}{u.role === 'paciente' && <option value="paciente">Paciente</option>}</select></td>
        <td><span className={`badge ${u.active ? 'success' : 'pending'}`}>{u.active ? 'Activo' : 'Inactivo'}</span></td>
        <td>{u.id !== currentUserId && <button className="text-button" onClick={() => void save(u, { active: !u.active })}>{u.active ? 'Desactivar' : 'Activar'}</button>}
          <button className="text-button" onClick={() => { const p = window.prompt(`Nueva contraseña para ${u.full_name} (mínimo 8 caracteres):`); if (p) void run(() => post(`/users/${u.id}/reset-password`, { password: p }), 'Contraseña restablecida') }}>Cambiar clave</button>
          <button className="text-button" onClick={() => { if (window.confirm(`¿Restablecer el MFA de ${u.full_name}? Deberá configurarlo de nuevo.`)) void run(() => post(`/users/${u.id}/reset-mfa`, {}), 'MFA restablecido') }}>Resetear MFA</button></td></tr>)}
    </tbody></table></div>
  </section>
}

type RecordData = {
  patient: { id: number; first_name: string; last_name: string; document_id: string; birth_date: string | null; phone: string; email: string; insurance: string; emergency_contact: string; emergency_phone: string; guardian_name: string; medical_history: string; allergies: string; medical_alerts: string }
  alerts: string[]; appointments: Row[]; treatment_plans: (Row & { items: string })[]; treatments: Row[]; invoices: Row[]
  counts: { clinical_notes: number; consents: number; files: number }
}

export function PatientRecordView({ patientId }: { patientId: number }) {
  const [rec, setRec] = useState<RecordData | null>(null), [error, setError] = useState('')
  useEffect(() => { request<RecordData>(`/patients/${patientId}/record`).then(setRec).catch(e => setError(e instanceof Error ? e.message : 'No se pudo cargar el expediente')) }, [patientId])
  const exportJson = async () => { try { saveBlob(await downloadFile(`/patients/${patientId}/record?export=1`), `expediente-${patientId}.json`) } catch (e) { setError(e instanceof Error ? e.message : 'No se pudo exportar') } }
  if (error) return <div className="alert-error">{error}</div>
  if (!rec) return <div className="empty-inline">Cargando expediente…</div>
  const p = rec.patient
  return <div>
    <div className="panel-head"><div><h2>{p.first_name} {p.last_name}</h2><p>{p.document_id || 'Sin documento'} · {p.birth_date ?? 'Sin fecha de nacimiento'} · {p.insurance || 'Particular'}</p></div>
      <div><button className="secondary" onClick={() => window.print()}>Imprimir</button> <button className="primary" onClick={() => void exportJson()}>Exportar (auditado)</button></div></div>
    {rec.alerts.length > 0 && <div className="alert-error" role="alert"><strong>ALERTAS MÉDICAS:</strong> {rec.alerts.join(' · ')}</div>}
    <p><b>Contacto:</b> {p.phone || '—'} · {p.email || '—'} · <b>Emergencia:</b> {p.emergency_contact || '—'} {p.emergency_phone} {p.guardian_name && <>· <b>Tutor:</b> {p.guardian_name}</>}</p>
    <p><b>Anamnesis:</b> {p.medical_history || '—'}</p>
    <p className="muted">Notas SOEP: {rec.counts.clinical_notes} · Consentimientos: {rec.counts.consents} · Archivos: {rec.counts.files} (se gestionan en «Expediente clínico»)</p>
    <h3>Citas</h3><DataTable columns={['starts_at', 'servicio', 'profesional', 'sillon', 'status']} rows={rec.appointments} empty="Sin citas." />
    <h3>Planes de tratamiento</h3>{rec.treatment_plans.length === 0 ? <div className="empty-inline">Sin planes.</div> : rec.treatment_plans.map(pl => {
      const items = JSON.parse(pl.items) as Row[]
      return <div key={String(pl.id)}><strong>{String(pl.title)}</strong> <span className="badge pending">{String(pl.status)}</span>
        <DataTable columns={['fase', 'sesion', 'servicio', 'estado', 'honorario', 'laboratorio', 'descuento']} rows={items} empty="Sin ítems." /></div>
    })}
    <h3>Tratamientos realizados</h3><DataTable columns={['performed_at', 'servicio', 'profesional']} rows={rec.treatments} empty="Aún no hay tratamientos realizados." />
    <h3>Cobros</h3><DataTable columns={['created_at', 'total', 'pagado', 'status']} rows={rec.invoices} empty="Sin cobros." />
  </div>
}
