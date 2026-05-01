import { useAuthStore } from '@/store'
import { queryClient } from '@/lib/queryClient'

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

export class BillingNotConfigured extends Error {
  constructor() {
    super('Billing service is not configured (self-host build)')
    this.name = 'BillingNotConfigured'
  }
}

let logoutPromise: Promise<void> | null = null

type Origin = 'processing' | 'billing'

interface RequestOpts {
  baseUrl?: string
  origin: Origin
}

async function request<T>(method: string, path: string, body: unknown, opts: RequestOpts): Promise<T> {
  const url = opts.baseUrl ? `${opts.baseUrl}${path}` : path

  const headers: Record<string, string> = {}
  if (opts.origin === 'processing') headers['X-Requested-With'] = 'XMLHttpRequest'
  if (body) headers['Content-Type'] = 'application/json'

  const res = await fetch(url, {
    method,
    headers,
    // 'include' for cross-origin (billing); 'same-origin' for processing.
    credentials: opts.origin === 'billing' ? 'include' : 'same-origin',
    body: body ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    // 401 from processing -> hard logout (session expired).
    // 401 from billing -> surface to caller; UI handles it (e.g. as
    // "not requested yet" when the JWT cookie hasn't propagated yet).
    if (opts.origin === 'processing') {
      if (!logoutPromise) {
        logoutPromise = (async () => {
          try {
            await fetch('/auth/logout', {
              method: 'POST',
              credentials: 'same-origin',
              headers: { 'X-Requested-With': 'XMLHttpRequest' },
            })
          } catch {
            /* ignore */
          }
          useAuthStore.getState().logout()
          window.location.href = '/login'
        })().finally(() => {
          logoutPromise = null
        })
      }
      await logoutPromise
      throw new ApiError(401, 'unauthorized', 'Session expired')
    }
    throw new ApiError(401, 'unauthorized', 'Authentication required')
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { code: 'unknown', message: `HTTP ${res.status}` } }))
    throw new ApiError(res.status, err?.error?.code ?? 'unknown', err?.error?.message ?? `HTTP ${res.status}`)
  }

  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path, undefined, { origin: 'processing' }),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body, { origin: 'processing' }),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body, { origin: 'processing' }),
  delete: <T>(path: string) => request<T>('DELETE', path, undefined, { origin: 'processing' }),
}

// Reads the billing URL from the /meta cache without importing the hook
// module (avoids a circular dep with use-meta -> api -> use-meta).
function billingBaseUrl(): string {
  const meta = queryClient.getQueryData<{ billing_url?: string }>(['meta'])
  const url = meta?.billing_url
  if (!url) throw new BillingNotConfigured()
  return url
}

function billingRequest<T>(method: string, path: string, body?: unknown): Promise<T> {
  return request<T>(method, path, body, { origin: 'billing', baseUrl: billingBaseUrl() })
}

export const billingApi = {
  get: <T>(path: string) => billingRequest<T>('GET', path),
  post: <T>(path: string, body?: unknown) => billingRequest<T>('POST', path, body),
}
