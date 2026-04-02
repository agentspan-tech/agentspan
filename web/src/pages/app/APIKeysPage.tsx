import { useState, useEffect, useCallback } from 'react'
import { Copy, Check } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { useAuthStore } from '@/store'
import { useAPIKeys, useCreateAPIKey, useDeactivateAPIKey } from '@/hooks/use-keys'
import { useI18n } from '@/i18n'
import type { APIKeyCreateResult } from '@/types/api'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from '@/components/ui/dialog'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'

export function APIKeysPage() {
  const activeOrgID = useAuthStore((s) => s.activeOrgID) ?? ''
  const { data: keys, isLoading, isError } = useAPIKeys(activeOrgID)
  const createKey = useCreateAPIKey(activeOrgID)
  const deactivateKey = useDeactivateAPIKey(activeOrgID)
  const { t, tt } = useI18n()

  const PROVIDER_OPTIONS = [
    { label: 'OpenAI', value: 'openai' },
    { label: 'Anthropic', value: 'anthropic' },
    { label: 'DeepSeek', value: 'deepseek' },
    { label: 'Mistral', value: 'mistral' },
    { label: 'Groq', value: 'groq' },
    { label: 'Gemini', value: 'gemini' },
    { label: 'Custom', value: 'custom' },
  ]

  const [showCreate, setShowCreate] = useState(false)
  const [name, setName] = useState('')
  const [providerType, setProviderType] = useState('')
  const [providerKey, setProviderKey] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [createError, setCreateError] = useState('')
  const [rawKeyResult, setRawKeyResult] = useState<APIKeyCreateResult | null>(null)
  const [copied, setCopied] = useState(false)
  const [deactivateTarget, setDeactivateTarget] = useState<{ id: string; name: string } | null>(null)
  const [keyCountdown, setKeyCountdown] = useState(30)

  const dismissRawKey = useCallback(() => { setRawKeyResult(null); setCopied(false); setKeyCountdown(30) }, [])

  useEffect(() => {
    if (!rawKeyResult) return
    setKeyCountdown(30)
    const interval = setInterval(() => {
      setKeyCountdown((prev) => {
        if (prev <= 1) { clearInterval(interval); dismissRawKey(); return 0 }
        return prev - 1
      })
    }, 1000)
    return () => clearInterval(interval)
  }, [rawKeyResult, dismissRawKey])

  function resetCreateForm() { setName(''); setProviderType(''); setProviderKey(''); setBaseUrl(''); setCreateError('') }

  async function handleCreate() {
    if (!name.trim()) { setCreateError(t.keys_err_name_required); return }
    if (name.trim().length > 100) { setCreateError(t.keys_err_name_too_long); return }
    if (!providerType) { setCreateError(t.keys_err_provider_required); return }
    if (!providerKey.trim()) { setCreateError(t.keys_err_provider_key_required); return }
    if (providerKey.trim().length > 500) { setCreateError(t.keys_err_provider_key_too_long); return }
    if (baseUrl.trim()) {
      try { const u = new URL(baseUrl.trim()); if (!['http:', 'https:'].includes(u.protocol)) throw new Error() } catch { setCreateError(t.keys_err_base_url_invalid); return }
      if (baseUrl.trim().length > 500) { setCreateError(t.keys_err_base_url_too_long); return }
    }
    setCreateError('')
    try {
      const result = await createKey.mutateAsync({ name: name.trim(), provider_type: providerType, provider_key: providerKey.trim(), base_url: baseUrl.trim() || undefined })
      setShowCreate(false); resetCreateForm(); setRawKeyResult(result)
    } catch { setCreateError(t.keys_err_create_failed) }
  }

  function handleCopyRawKey() {
    if (!rawKeyResult) return
    navigator.clipboard.writeText(rawKeyResult.raw_key).catch(() => {})
    setCopied(true); setTimeout(() => setCopied(false), 2000)
  }

  async function handleDeactivateConfirm() {
    if (!deactivateTarget) return
    await deactivateKey.mutateAsync(deactivateTarget.id)
    setDeactivateTarget(null)
  }

  const inputClass = "w-full bg-zinc-900 border border-zinc-800 rounded-md px-3 py-2.5 text-sm text-zinc-100 placeholder:text-zinc-600 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 transition-colors"

  return (
    <div className="p-6 lg:p-8 animate-fade-in-up">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-xl font-semibold text-zinc-50 tracking-[-0.01em]">{t.keys_title}</h1>
          <p className="text-sm text-zinc-500 mt-1">{t.keys_subtitle}</p>
        </div>
        <button
          onClick={() => { resetCreateForm(); setShowCreate(true) }}
          className="text-sm font-medium bg-zinc-50 text-zinc-950 px-3.5 py-1.5 rounded-md hover:bg-zinc-200 transition-colors duration-150 btn-press"
        >{t.keys_create}</button>
      </div>

      {isLoading ? (
        <div className="space-y-2">{[1, 2, 3].map(i => <div key={i} className="h-12 skeleton-shimmer rounded-lg" />)}</div>
      ) : isError ? (
        <p className="text-sm text-zinc-500">{t.keys_failed_load}</p>
      ) : !keys || keys.length === 0 ? (
        <div className="text-center py-16">
          <div className="mb-5 mx-auto w-10 h-10 rounded-lg border border-zinc-800 bg-zinc-900 flex items-center justify-center">
            <div className="w-3 h-3 rounded-sm border border-dashed border-zinc-700" />
          </div>
          <p className="text-base font-medium text-zinc-200 mb-2">{t.keys_empty_title}</p>
          <p className="text-sm text-zinc-500 mb-6">{t.keys_empty_body}</p>
          <button onClick={() => { resetCreateForm(); setShowCreate(true) }} className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 transition-colors duration-150 btn-press">{t.keys_create}</button>
        </div>
      ) : (
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden animate-fade-in">
          <div className="overflow-x-auto">
            <table className="w-full text-sm text-left">
              <thead>
                <tr className="border-b border-zinc-800/60">
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.keys_col_name}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.keys_col_provider}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden md:table-cell">{t.keys_col_key}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.keys_col_status}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden sm:table-cell">{t.keys_col_last_used}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-800/40">
                {keys.map((key, i) => (
                  <tr key={key.id} className="table-row-hover animate-fade-in-up" style={{ animationDelay: `${Math.min(i * 30, 300)}ms` }}>
                    <td className="px-5 py-3.5 text-zinc-200 font-medium">{key.name}</td>
                    <td className="px-5 py-3.5 text-zinc-500 capitalize">{key.provider_type}</td>
                    <td className="px-5 py-3.5 font-mono text-zinc-600 hidden md:table-cell text-xs">{key.display}</td>
                    <td className="px-5 py-3.5">
                      {key.active
                        ? <span className="inline-flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wider text-emerald-400 bg-emerald-500/10 px-2 py-0.5 rounded"><span className="w-1 h-1 rounded-full bg-emerald-400" />{t.keys_status_active}</span>
                        : <span className="text-[10px] font-medium uppercase tracking-wider text-zinc-500 bg-zinc-800 px-2 py-0.5 rounded">{t.keys_status_inactive}</span>
                      }
                    </td>
                    <td className="px-5 py-3.5 text-zinc-600 hidden sm:table-cell">{key.last_used_at ? formatDistanceToNow(new Date(key.last_used_at), { addSuffix: true }) : t.keys_last_used_never}</td>
                    <td className="px-5 py-3.5">
                      {key.active && (
                        <button onClick={(e) => { e.stopPropagation(); setDeactivateTarget({ id: key.id, name: key.name }) }} className="text-sm text-red-400/80 hover:text-red-400 transition-colors">{t.keys_deactivate}</button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Create Dialog */}
      <Dialog open={showCreate} onOpenChange={(o) => { if (!o) { setShowCreate(false); resetCreateForm() } }}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-50">
          <DialogHeader><DialogTitle className="text-lg font-semibold">{t.keys_create_title}</DialogTitle></DialogHeader>
          <div className="space-y-4 py-2">
            <div><label className="block text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">{t.keys_create_name}</label><input placeholder={t.keys_create_name_placeholder} value={name} onChange={(e) => setName(e.target.value)} className={inputClass} /></div>
            <div><label className="block text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">{t.keys_create_provider}</label>
              <Select value={providerType} onValueChange={setProviderType}>
                <SelectTrigger className="bg-zinc-900 border-zinc-800 text-zinc-100"><SelectValue placeholder={t.keys_create_provider_placeholder} /></SelectTrigger>
                <SelectContent className="bg-zinc-900 border-zinc-800">{PROVIDER_OPTIONS.map(o => <SelectItem key={o.value} value={o.value} className="text-zinc-100">{o.label}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            <div><label className="block text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">{t.keys_create_provider_key}</label><input type="password" placeholder={t.keys_create_provider_key_placeholder} value={providerKey} onChange={(e) => setProviderKey(e.target.value)} className={inputClass} /></div>
            <div><label className="block text-xs font-medium text-zinc-500 uppercase tracking-wider mb-2">{t.keys_create_base_url} <span className="normal-case text-zinc-600">({t.keys_create_base_url_optional})</span></label><input placeholder={t.keys_create_base_url_placeholder} value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} className={inputClass} /></div>
            {createError && <p className="text-sm text-red-400">{createError}</p>}
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => { setShowCreate(false); resetCreateForm() }} className="text-zinc-400">{t.common_cancel}</Button>
            <button onClick={handleCreate} disabled={createKey.isPending} className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 transition-colors disabled:opacity-50 btn-press">{createKey.isPending ? t.keys_creating : t.keys_create_btn}</button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Raw Key Dialog */}
      <Dialog open={!!rawKeyResult} onOpenChange={(o) => { if (!o) dismissRawKey() }}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-50">
          <DialogHeader><DialogTitle className="text-lg font-semibold">{t.keys_raw_title}</DialogTitle></DialogHeader>
          <div className="py-2 space-y-4">
            <p className="text-sm text-zinc-400">{tt('keys_raw_body', { seconds: keyCountdown })}</p>
            <div className="relative">
              <div className="font-mono text-sm bg-zinc-950 border border-zinc-800 rounded-md p-3 select-all break-all text-zinc-100 pr-10">{rawKeyResult?.raw_key}</div>
              <button onClick={handleCopyRawKey} className="absolute right-2 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-200 transition-colors">
                {copied ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
              </button>
            </div>
          </div>
          <DialogFooter>
            <button onClick={dismissRawKey} className="text-sm font-medium bg-zinc-50 text-zinc-950 px-4 py-2 rounded-md hover:bg-zinc-200 transition-colors btn-press">{t.keys_raw_done}</button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Deactivate Dialog */}
      <Dialog open={!!deactivateTarget} onOpenChange={(o) => { if (!o) setDeactivateTarget(null) }}>
        <DialogContent className="bg-zinc-900 border-zinc-800 text-zinc-50">
          <DialogHeader><DialogTitle className="text-lg font-semibold">{t.keys_deactivate_title}</DialogTitle></DialogHeader>
          <p className="text-sm text-zinc-400 py-2">{t.keys_deactivate_body}</p>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDeactivateTarget(null)} className="text-zinc-400">{t.common_cancel}</Button>
            <button onClick={handleDeactivateConfirm} disabled={deactivateKey.isPending} className="text-sm font-medium bg-red-500/10 text-red-400 px-4 py-2 rounded-md hover:bg-red-500/20 transition-colors disabled:opacity-50">{deactivateKey.isPending ? t.keys_deactivating : t.keys_deactivate_confirm}</button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
