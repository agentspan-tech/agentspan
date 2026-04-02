import { useEffect, useRef } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'

export function VerifyEmailPage() {
  const { t } = useI18n()
  const [searchParams] = useSearchParams(); const token = searchParams.get('token') ?? ''
  const verifyMutation = useMutation({ mutationFn: () => api.post('/auth/verify-email', { token }) })
  const calledRef = useRef(false)
  useEffect(() => { if (token && !calledRef.current) { calledRef.current = true; verifyMutation.mutate() } }, [token]) // eslint-disable-line react-hooks/exhaustive-deps -- intentional: fire once on mount

  if (!token) return <div className="min-h-screen flex items-center justify-center bg-zinc-950 px-6 auth-grid-bg"><div className="text-center"><p className="text-sm text-zinc-400 mb-4">{t.auth_verify_token_invalid}</p><Link to="/login" className="text-sm text-zinc-500 hover:text-zinc-300">{t.auth_verify_back}</Link></div></div>

  return (
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 px-6 auth-grid-bg">
      <div className="w-full max-w-sm text-center">
        <div className="mb-6"><span className="text-sm font-semibold text-zinc-50 tracking-tight">AgentSpan</span></div>
        {verifyMutation.isPending && <div className="flex items-center justify-center gap-2 text-zinc-400"><Loader2 size={16} className="animate-spin" /><span className="text-sm">{t.auth_verify_verifying}</span></div>}
        {verifyMutation.isSuccess && <div className="space-y-4"><p className="text-sm text-zinc-200">{t.auth_verify_success}</p><Link to="/login" className="inline-flex w-full justify-center bg-zinc-50 text-zinc-950 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-200 transition-colors">{t.auth_login_btn}</Link></div>}
        {verifyMutation.isError && <div className="space-y-4"><p className="text-sm text-zinc-400">{t.auth_verify_failed}</p><Link to="/login" className="text-sm text-zinc-500 hover:text-zinc-300 block">{t.auth_verify_back}</Link></div>}
      </div>
    </div>
  )
}
