import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'

const inputClass = "w-full bg-zinc-900 border border-zinc-800 rounded-md px-3 py-2.5 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 transition-colors"

export function ResetPasswordPage() {
  const { t } = useI18n()
  const [searchParams] = useSearchParams(); const token = searchParams.get('token') ?? ''
  const [password, setPassword] = useState(''); const [confirmPassword, setConfirmPassword] = useState('')
  const [fieldErrors, setFieldErrors] = useState<{ password?: string; confirmPassword?: string }>({}); const [formError, setFormError] = useState<string | null>(null); const [success, setSuccess] = useState(false)
  const resetMutation = useMutation({ mutationFn: () => api.post('/auth/reset-password', { token, password }), onSuccess: () => setSuccess(true), onError: (err: unknown) => setFormError(err instanceof ApiError ? err.message || t.auth_something_wrong : t.auth_something_wrong) })
  function validate() { const e: typeof fieldErrors = {}; if (!password) e.password = t.auth_field_required; else if (password.length < 8) e.password = t.auth_password_min; if (!confirmPassword) e.confirmPassword = t.auth_field_required; else if (password !== confirmPassword) e.confirmPassword = t.auth_passwords_mismatch; setFieldErrors(e); return Object.keys(e).length === 0 }
  function handleSubmit(e: React.FormEvent) { e.preventDefault(); setFormError(null); if (!validate()) return; resetMutation.mutate() }

  if (!token) return <div className="min-h-screen flex items-center justify-center bg-zinc-950 px-6 auth-grid-bg"><div className="text-center"><p className="text-sm text-zinc-400 mb-4">{t.auth_reset_token_invalid}</p><Link to="/request-password-reset" className="text-sm text-zinc-500 hover:text-zinc-300">{t.auth_reset_request_new}</Link></div></div>

  return (
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 px-6 auth-grid-bg">
      <div className="w-full max-w-sm">
        <div className="mb-8"><span className="text-sm font-semibold text-zinc-50 tracking-tight">AgentSpan</span></div>
        {success ? (
          <div className="space-y-4 text-center"><p className="text-sm text-zinc-200">{t.auth_reset_success}</p><Link to="/login" className="inline-flex w-full justify-center bg-zinc-50 text-zinc-950 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-200 transition-colors">{t.auth_login_btn}</Link></div>
        ) : (<>
          <h1 className="text-2xl font-semibold tracking-tight text-zinc-50 mb-6">{t.auth_new_password_title}</h1>
          <form onSubmit={handleSubmit} noValidate className="space-y-4">
            <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.auth_new_password}</label><input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" className={inputClass} />{fieldErrors.password && <p className="mt-1.5 text-sm text-red-400">{fieldErrors.password}</p>}</div>
            <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.auth_confirm_password}</label><input type="password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} autoComplete="new-password" className={inputClass} />{fieldErrors.confirmPassword && <p className="mt-1.5 text-sm text-red-400">{fieldErrors.confirmPassword}</p>}</div>
            {formError && <p className="text-sm text-red-400">{formError}</p>}
            <button type="submit" disabled={resetMutation.isPending} className="w-full bg-zinc-50 text-zinc-950 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-200 transition-colors duration-150 flex items-center justify-center gap-2 disabled:opacity-50">{resetMutation.isPending && <Loader2 size={14} className="animate-spin" />}{t.auth_reset_btn}</button>
          </form>
        </>)}
      </div>
    </div>
  )
}
