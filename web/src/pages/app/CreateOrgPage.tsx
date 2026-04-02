import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { api, ApiError } from '@/lib/api'
import { useAuthStore } from '@/store'
import { useI18n } from '@/i18n'
import type { Organization } from '@/types/api'

const inputClass = "w-full bg-zinc-900 border border-zinc-800 rounded-md px-3 py-2.5 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 transition-colors"

export function CreateOrgPage() {
  const navigate = useNavigate()
  const { setActiveOrgID } = useAuthStore()
  const { t } = useI18n()
  const [orgName, setOrgName] = useState(() => sessionStorage.getItem('pending_org_name') ?? '')
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [formError, setFormError] = useState<string | null>(null)

  const createOrgMutation = useMutation({
    mutationFn: () => api.post<Organization>('/api/orgs/', { name: orgName }),
    onSuccess: (org) => { setActiveOrgID(org.id); sessionStorage.removeItem('pending_org_name'); navigate('/dash') },
    onError: (err: unknown) => { setFormError(err instanceof ApiError ? err.message || t.auth_something_wrong : t.auth_something_wrong) },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault(); setFormError(null)
    if (!orgName.trim()) { setFieldError(t.create_org_required); return }
    setFieldError(null); createOrgMutation.mutate()
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-zinc-950 px-6 auth-grid-bg">
      <div className="w-full max-w-sm">
        <div className="mb-8">
          <span className="text-sm font-semibold text-zinc-50 tracking-tight">AgentSpan</span>
          <h1 className="mt-6 text-2xl font-semibold tracking-tight text-zinc-50">{t.create_org_title}</h1>
          <p className="mt-1.5 text-sm text-zinc-500">{t.create_org_subtitle}</p>
        </div>
        <form onSubmit={handleSubmit} noValidate className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-zinc-300 mb-1.5">{t.create_org_name}</label>
            <input type="text" value={orgName} onChange={(e) => setOrgName(e.target.value)} placeholder={t.create_org_placeholder} autoFocus className={inputClass} />
            {fieldError && <p className="mt-1.5 text-sm text-red-400">{fieldError}</p>}
          </div>
          {formError && <p className="text-sm text-red-400">{formError}</p>}
          <button type="submit" disabled={createOrgMutation.isPending} className="w-full bg-zinc-50 text-zinc-950 py-2.5 rounded-md text-sm font-medium hover:bg-zinc-200 transition-colors duration-150 flex items-center justify-center gap-2 disabled:opacity-50">
            {createOrgMutation.isPending && <Loader2 size={14} className="animate-spin" />}
            {t.create_org_btn}
          </button>
        </form>
      </div>
    </div>
  )
}
