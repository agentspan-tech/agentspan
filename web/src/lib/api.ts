import { useAuthStore } from '@/store'

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

let logoutPromise: Promise<void> | null = null

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'X-Requested-With': 'XMLHttpRequest' }
  if (body) headers['Content-Type'] = 'application/json'

  const res = await fetch(path, {
    method,
    headers,
    credentials: 'same-origin',
    body: body ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    if (!logoutPromise) {
      logoutPromise = (async () => {
        try {
          await fetch('/auth/logout', { method: 'POST', credentials: 'same-origin', headers: { 'X-Requested-With': 'XMLHttpRequest' } })
        } catch { /* ignore */ }
        useAuthStore.getState().logout()
        window.location.href = '/login'
      })().finally(() => { logoutPromise = null })
    }
    await logoutPromise
    throw new ApiError(401, 'unauthorized', 'Session expired')
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { code: 'unknown', message: `HTTP ${res.status}` } }))
    throw new ApiError(res.status, err?.error?.code ?? 'unknown', err?.error?.message ?? `HTTP ${res.status}`)
  }

  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  delete: <T>(path: string) => request<T>('DELETE', path),
}
