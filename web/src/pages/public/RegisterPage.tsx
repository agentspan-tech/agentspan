import { useState, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Copy, Check, Loader2 } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { useI18n } from '@/i18n'
import type { RegisterResponse, SetupStatusResponse } from '@/types/api'

const inputClass = "w-full bg-zinc-900 border border-zinc-800 rounded-md px-3 py-2.5 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 transition-colors"

type RegisterResult = { kind: 'email_sent'; email: string } | { kind: 'verification_url'; email: string; url: string }

export function RegisterPage() {
  const navigate = useNavigate()
  const { t } = useI18n()
  const [name, setName] = useState(''); const [email, setEmail] = useState(''); const [password, setPassword] = useState(''); const [orgName, setOrgName] = useState('')
  const [fieldErrors, setFieldErrors] = useState<{ name?: string; email?: string; password?: string; orgName?: string; terms?: string; consent?: string }>({})
  const [acceptedTerms, setAcceptedTerms] = useState(false); const [acceptedConsent, setAcceptedConsent] = useState(false)
  const [result, setResult] = useState<RegisterResult | null>(null); const [copied, setCopied] = useState(false)

  // Check if registration is open (self-host: only before first user)
  const { data: setupStatus, isLoading: setupLoading } = useQuery({
    queryKey: ['setup-status'],
    queryFn: () => api.get<SetupStatusResponse>('/auth/setup-status'),
  })

  useEffect(() => {
    if (setupStatus && !setupStatus.registration_open) {
      navigate('/login', { replace: true })
    }
  }, [setupStatus, navigate])

  const registerMutation = useMutation({
    mutationFn: () => api.post<RegisterResponse>('/auth/register', { email, name, password, organization_name: orgName.trim(), accepted_terms: true, accepted_privacy: true }),
    onSuccess: (data) => {
      // Self-host first user: auto-verified, go straight to login
      if (data.auto_login) {
        navigate('/login', { replace: true, state: { registered: true } })
        return
      }
      setResult(data.verification_url ? { kind: 'verification_url', email: data.email, url: data.verification_url } : { kind: 'email_sent', email: data.email })
    },
    onError: (err: unknown) => {
      if (err instanceof ApiError && err.code === 'registration_closed') {
        navigate('/login', { replace: true })
        return
      }
      if (err instanceof ApiError && (err.status === 409 || err.code === 'email_exists')) {
        setFieldErrors(prev => ({ ...prev, email: t.auth_email_exists }))
        return
      }
      setFieldErrors(prev => ({ ...prev, email: t.auth_something_wrong }))
    },
  })

  function validate() {
    const errors: typeof fieldErrors = {}
    if (!name.trim()) errors.name = t.auth_field_required
    if (!email.trim()) errors.email = t.auth_field_required
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) errors.email = t.auth_email_invalid
    if (!password) errors.password = t.auth_field_required
    else if (password.length < 8) errors.password = t.auth_password_min
    else if (!/[a-z]/.test(password) || !/[A-Z]/.test(password) || !/[0-9]/.test(password)) errors.password = t.auth_password_rules
    if (!orgName.trim()) errors.orgName = t.auth_field_required
    if (!acceptedTerms) errors.terms = t.auth_register_must_accept
    if (!acceptedConsent) errors.consent = t.auth_register_must_accept
    setFieldErrors(errors)
    return Object.keys(errors).length === 0
  }

  function handleSubmit(e: React.FormEvent) { e.preventDefault(); setFieldErrors({}); if (!validate()) return; registerMutation.mutate() }
  async function handleCopy(url: string) { try { await navigator.clipboard.writeText(url) } catch { /* clipboard unavailable */ } setCopied(true); setTimeout(() => setCopied(false), 2000) }

  // While checking setup status, show nothing (avoids flash)
  if (setupLoading) return null

  if (result) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-zinc-950 px-6 auth-grid-bg">
        <div className="w-full max-w-sm text-center">
          <h1 className="text-2xl font-semibold tracking-tight text-zinc-50 mb-2">{t.auth_register_check_email}</h1>
          {result.kind === 'email_sent' ? (
            <><p className="text-sm text-zinc-500 mb-1">{t.auth_register_sent_to}</p><p className="text-sm text-zinc-300 font-medium mb-8">{result.email}</p></>
          ) : (
            <div className="space-y-4 text-left">
              <p className="text-sm text-zinc-400 text-center">{t.auth_register_copy_link}</p>
              <div className="relative"><div className="bg-zinc-900 border border-zinc-800 rounded-md px-3 py-2 pr-10 font-mono text-sm text-zinc-100 break-all">{result.url}</div>
                <button onClick={() => handleCopy(result.url)} className="absolute right-2 top-2 text-zinc-500 hover:text-zinc-200">{copied ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}</button>
              </div>
            </div>
          )}
          <div className="mt-8 pt-6 border-t border-zinc-800"><Link to="/login" className="text-sm text-zinc-500 hover:text-zinc-300 transition-colors">{t.auth_register_back_login}</Link></div>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 px-6 auth-grid-bg">
      <div className="w-full max-w-sm">
        <div className="mb-8">
          <Link to="/" className="flex items-center gap-2"><span className="text-sm font-semibold text-zinc-50 tracking-tight">AgentOrbit</span></Link>
          <h1 className="mt-6 text-2xl font-semibold tracking-tight text-zinc-50">{t.auth_register_title}</h1>
          <p className="mt-1.5 text-sm text-zinc-500">{t.auth_register_subtitle}</p>
        </div>
        <form onSubmit={handleSubmit} noValidate className="space-y-4">
          {[
            { id: 'name', label: t.auth_register_name, type: 'text', val: name, set: setName, ac: 'name', err: fieldErrors.name },
            { id: 'email', label: t.auth_register_email, type: 'email', val: email, set: setEmail, ac: 'email', err: fieldErrors.email },
            { id: 'password', label: t.auth_register_password, type: 'password', val: password, set: setPassword, ac: 'new-password', err: fieldErrors.password },
            { id: 'orgName', label: t.auth_register_org, type: 'text', val: orgName, set: setOrgName, ac: 'organization', err: fieldErrors.orgName },
          ].map(f => (
            <div key={f.id}>
              <label className="block text-sm font-medium text-zinc-300 mb-1.5">{f.label}</label>
              <input type={f.type} value={f.val} onChange={(e) => f.set(e.target.value)} autoComplete={f.ac} className={inputClass} />
              {f.err && <p className="mt-1.5 text-sm text-red-400">{f.err}</p>}
            </div>
          ))}
          <div className="space-y-3 pt-1">
            <label className="flex items-start gap-2.5 cursor-pointer">
              <input type="checkbox" checked={acceptedTerms} onChange={(e) => setAcceptedTerms(e.target.checked)} className="mt-0.5 w-4 h-4 rounded border-zinc-700 bg-zinc-900 text-indigo-500 focus:ring-indigo-500/30 focus:ring-offset-0 shrink-0" />
              <span className="text-xs text-zinc-400 leading-relaxed">
                {t.auth_register_accept_terms
                  .split('{terms}')[0]}
                <a href="/terms" target="_blank" className="text-zinc-200 underline underline-offset-2 hover:text-zinc-50">{t.auth_register_terms_link}</a>
                {t.auth_register_accept_terms
                  .split('{terms}')[1]?.split('{privacy}')[0]}
                <a href="/privacy-policy" target="_blank" className="text-zinc-200 underline underline-offset-2 hover:text-zinc-50">{t.auth_register_privacy_link}</a>
                {t.auth_register_accept_terms
                  .split('{privacy}')[1] || ''}
              </span>
            </label>
            {fieldErrors.terms && <p className="text-sm text-red-400 ml-6">{fieldErrors.terms}</p>}
            <label className="flex items-start gap-2.5 cursor-pointer">
              <input type="checkbox" checked={acceptedConsent} onChange={(e) => setAcceptedConsent(e.target.checked)} className="mt-0.5 w-4 h-4 rounded border-zinc-700 bg-zinc-900 text-indigo-500 focus:ring-indigo-500/30 focus:ring-offset-0 shrink-0" />
              <span className="text-xs text-zinc-400 leading-relaxed">
                {t.auth_register_accept_consent
                  .split('{consent}')[0]}
                <a href="/consent" target="_blank" className="text-zinc-200 underline underline-offset-2 hover:text-zinc-50">{t.auth_register_consent_link}</a>
                {t.auth_register_accept_consent
                  .split('{consent}')[1] || ''}
              </span>
            </label>
            {fieldErrors.consent && <p className="text-sm text-red-400 ml-6">{fieldErrors.consent}</p>}
          </div>
          <button type="submit" disabled={registerMutation.isPending} className="w-full bg-zinc-50 text-zinc-950 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-200 transition-colors duration-150 flex items-center justify-center gap-2 mt-2 disabled:opacity-50">
            {registerMutation.isPending && <Loader2 size={14} className="animate-spin" />}
            {t.auth_register_btn}
          </button>
        </form>
      </div>
    </div>
  )
}
