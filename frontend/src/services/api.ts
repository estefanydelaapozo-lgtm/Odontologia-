const API = import.meta.env.VITE_API_URL || '/api'
export const token = () => localStorage.getItem('sigo_token') ?? ''
export async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  headers.set('Content-Type', 'application/json')
  if (token()) headers.set('Authorization', `Bearer ${token()}`)
  const res = await fetch(`${API}${path}`, { ...options, headers })
  const renewed = res.headers.get('X-Session-Token')
  if (renewed) localStorage.setItem('sigo_token', renewed)
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.message ?? `Error ${res.status}`)
  return data as T
}
export const post = <T = unknown>(path: string, body: unknown) => request<T>(path, { method: 'POST', body: JSON.stringify(body) })
export const put = (path: string, body: unknown) => request(path, { method: 'PUT', body: JSON.stringify(body) })
export const patch = (path: string, body: unknown) => request(path, { method: 'PATCH', body: JSON.stringify(body) })
export async function uploadFile<T>(path: string, file: File): Promise<T> {
  const body = new FormData(); body.set('file', file)
  const res = await fetch(`${API}${path}`, { method: 'POST', body, headers: { Authorization: `Bearer ${token()}` } })
  const renewed = res.headers.get('X-Session-Token'); if (renewed) localStorage.setItem('sigo_token', renewed)
  const data = await res.json().catch(() => ({})); if (!res.ok) throw new Error(data.message ?? `Error ${res.status}`)
  return data as T
}
export async function downloadFile(path: string): Promise<Blob> {
  const res = await fetch(`${API}${path}`, { headers: { Authorization: `Bearer ${token()}` } })
  const renewed = res.headers.get('X-Session-Token'); if (renewed) localStorage.setItem('sigo_token', renewed)
  if (!res.ok) { const data = await res.json().catch(() => ({})); throw new Error(data.message ?? `Error ${res.status}`) }
  return res.blob()
}
