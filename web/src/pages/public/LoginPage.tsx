import { useState } from 'react'
import { useNavigate, useLocation, Link } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { apiErrorMessage } from '@/lib/apiErrorMessage'
import { useAuthStore } from '@/store'
import { useI18n } from '@/i18n'
import type { LoginResponse, Organization, PaginatedResponse, SetupStatusResponse } from '@/types/api'

const inputClass = "w-full bg-zinc-900 border border-zinc-800 rounded-md px-3 py-2.5 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 transition-colors"

export function LoginPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const justRegistered = (location.state as { registered?: boolean })?.registered
  const { setAuthenticated, setActiveOrgID } = useAuthStore()
  const { t } = useI18n()

  const { data: setupStatus } = useQuery({
    queryKey: ['setup-status'],
    queryFn: () => api.get<SetupStatusResponse>('/auth/setup-status'),
  })
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [fieldErrors, setFieldErrors] = useState<{ email?: string; password?: string }>({})
  const [formError, setFormError] = useState<{ code: string; message: string } | null>(null)

  const resendMutation = useMutation({
    mutationFn: async () => api.post('/auth/resend-verification', { email }),
  })

  const loginMutation = useMutation({
    mutationFn: async () => api.post<LoginResponse>('/auth/login', { email, password }),
    onSuccess: async () => {
      setAuthenticated(true)
      try {
        const orgs = await api.get<Organization[] | PaginatedResponse<Organization>>('/api/orgs/')
        const orgList = Array.isArray(orgs) ? orgs : orgs.data
        if (orgList.length === 0) navigate('/create-org')
        else {
          setActiveOrgID(orgList[0].id)
          // Sync org locale to i18n store (D-06)
          const orgLocale = (orgList[0] as Organization & { locale?: string }).locale
          if (orgLocale === 'ru' || orgLocale === 'en') {
            useI18n.getState().setLang(orgLocale)
          }
          navigate('/dash')
        }
      } catch (err) {
        if (err instanceof ApiError && err.status === 404) {
          navigate('/create-org')
        } else {
          setFormError({ code: 'org_load_error', message: t.auth_login_org_error })
        }
      }
    },
    onError: (err: unknown) => {
      const code = err instanceof ApiError ? err.code : 'unknown'
      setFormError({ code, message: apiErrorMessage(err, t) })
    },
  })

  function validate() {
    const errors: { email?: string; password?: string } = {}
    if (!email.trim()) errors.email = t.auth_login_field_required
    if (!password) errors.password = t.auth_login_field_required
    setFieldErrors(errors)
    return Object.keys(errors).length === 0
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault(); setFormError(null)
    if (!validate()) return
    loginMutation.mutate()
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 px-6 auth-grid-bg">
      <div className="w-full max-w-sm">
        <div className="mb-8">
          <Link to="/" className="flex items-center gap-2">
            <span className="text-sm font-semibold text-zinc-50 tracking-tight">AgentOrbit</span>
          </Link>
          <h1 className="mt-6 text-2xl font-semibold tracking-[-0.02em] text-zinc-50">{t.auth_login_title}</h1>
          {setupStatus?.registration_open ? (
            <p className="mt-1.5 text-sm text-zinc-500">
              {!setupStatus.setup_complete ? t.auth_login_first_time : t.auth_login_subtitle}{' '}
              <Link to="/register" className="text-zinc-300 hover:text-zinc-50 transition-colors">{t.auth_login_create_account}</Link>
            </p>
          ) : (
            <p className="mt-1.5 text-sm text-zinc-500">{t.auth_login_subtitle}</p>
          )}
        </div>

        <form onSubmit={handleSubmit} noValidate className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.auth_login_email}</label>
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" className={inputClass} />
            {fieldErrors.email && <p className="mt-1.5 text-sm text-red-400">{fieldErrors.email}</p>}
          </div>
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="block text-sm font-medium text-zinc-300">{t.auth_login_password}</label>
              <Link to="/request-password-reset" className="text-sm text-zinc-500 hover:text-zinc-300 transition-colors">{t.auth_login_forgot}</Link>
            </div>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" className={inputClass} />
            {fieldErrors.password && <p className="mt-1.5 text-sm text-red-400">{fieldErrors.password}</p>}
          </div>
          {justRegistered && <p className="text-sm text-emerald-400">{t.auth_login_registered}</p>}
          {formError && (
            <div className="space-y-2">
              <p className="text-sm text-red-400">{formError.message}</p>
              {formError.code === 'email_not_verified' && email && (
                <button
                  type="button"
                  onClick={() => resendMutation.mutate()}
                  disabled={resendMutation.isPending || resendMutation.isSuccess}
                  className="text-sm text-zinc-300 hover:text-zinc-50 underline underline-offset-2 disabled:opacity-70 disabled:no-underline"
                >
                  {resendMutation.isSuccess
                    ? t.auth_resend_verification_sent
                    : resendMutation.isPending
                      ? t.auth_resend_verification_sending
                      : t.auth_resend_verification_btn}
                </button>
              )}
            </div>
          )}
          <button type="submit" disabled={loginMutation.isPending} className="w-full bg-zinc-50 text-zinc-950 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-200 transition-colors duration-150 flex items-center justify-center gap-2 mt-2 disabled:opacity-50">
            {loginMutation.isPending && <Loader2 size={14} className="animate-spin" />}
            {t.auth_login_btn}
          </button>
        </form>
      </div>
    </div>
  )
}
