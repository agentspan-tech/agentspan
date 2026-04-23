import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'

const inputClass = "w-full bg-zinc-900 border border-zinc-800 rounded-md px-3 py-2.5 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 transition-colors"

export function RequestPasswordResetPage() {
  const { t } = useI18n()
  const [email, setEmail] = useState(''); const [fieldError, setFieldError] = useState<string | null>(null); const [submitted, setSubmitted] = useState(false)
  const resetMutation = useMutation({ mutationFn: () => api.post('/auth/request-password-reset', { email }), onSuccess: () => setSubmitted(true), onError: () => setSubmitted(true) })
  function handleSubmit(e: React.FormEvent) { e.preventDefault(); if (!email.trim()) { setFieldError(t.auth_field_required); return }; setFieldError(null); resetMutation.mutate() }

  return (
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 px-6 auth-grid-bg">
      <div className="w-full max-w-sm">
        <div className="mb-8"><Link to="/" className="text-sm font-semibold text-zinc-50 tracking-tight">AgentOrbit</Link>
          {submitted ? (
            <div className="mt-6 space-y-4"><p className="text-sm text-zinc-400">{t.auth_reset_sent}</p><Link to="/login" className="text-sm text-zinc-500 hover:text-zinc-300 transition-colors">{t.auth_reset_back}</Link></div>
          ) : (<>
            <h1 className="mt-6 text-2xl font-semibold tracking-tight text-zinc-50">{t.auth_reset_title}</h1>
            <form onSubmit={handleSubmit} noValidate className="mt-6 space-y-4">
              <div><label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.auth_reset_email}</label><input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" className={inputClass} />{fieldError && <p className="mt-1.5 text-sm text-red-400">{fieldError}</p>}</div>
              <button type="submit" disabled={resetMutation.isPending} className="w-full bg-zinc-50 text-zinc-950 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-200 transition-colors duration-150 flex items-center justify-center gap-2 disabled:opacity-50">{resetMutation.isPending && <Loader2 size={14} className="animate-spin" />}{t.auth_reset_send}</button>
            </form>
            <div className="mt-4"><Link to="/login" className="text-sm text-zinc-500 hover:text-zinc-300 transition-colors">{t.auth_reset_back}</Link></div>
          </>)}
        </div>
      </div>
    </div>
  )
}
