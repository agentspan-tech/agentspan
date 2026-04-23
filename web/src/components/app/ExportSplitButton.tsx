import { useState, useRef, useEffect } from 'react'
import { useI18n } from '@/i18n'
import { Download, ChevronDown } from 'lucide-react'

interface ExportSplitButtonProps {
  orgID: string
  filters: Record<string, string | undefined>
}

export function ExportSplitButton({ orgID, filters }: ExportSplitButtonProps) {
  const { t } = useI18n()
  const [level, setLevel] = useState<'sessions' | 'spans'>('sessions')
  const [open, setOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    if (open) {
      document.addEventListener('mousedown', handleClickOutside)
      return () => document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [open])

  function handleExport() {
    const params = new URLSearchParams()
    params.set('format', 'csv')
    params.set('level', level)
    if (filters.status) params.set('status', filters.status)
    if (filters.agent_name) params.set('agent_name', filters.agent_name)
    if (filters.api_key_id) params.set('api_key_id', filters.api_key_id)
    if (filters.provider_type) params.set('provider_type', filters.provider_type)

    const now = new Date()
    const from = new Date(now)
    from.setDate(from.getDate() - 30)
    params.set('from', from.toISOString())
    params.set('to', now.toISOString())

    window.location.assign(`/api/orgs/${orgID}/sessions/export?${params.toString()}`)
  }

  return (
    <div className="relative" ref={dropdownRef}>
      <div className="inline-flex items-center">
        <button
          onClick={handleExport}
          className="inline-flex items-center gap-1.5 bg-zinc-900 border border-zinc-800 rounded-l-md px-3 py-1.5 text-sm text-zinc-300 hover:bg-zinc-800 hover:border-zinc-700 transition-colors"
        >
          <Download size={13} />
          {t.sessions_export}
        </button>
        <button
          onClick={() => setOpen(!open)}
          className="inline-flex items-center gap-1 bg-zinc-900 border border-zinc-800 border-l-0 rounded-r-md px-2 py-1.5 text-sm text-zinc-400 hover:bg-zinc-800 hover:border-zinc-700 transition-colors"
        >
          {level === 'sessions' ? t.sessions_export_sessions : t.sessions_export_spans}
          <ChevronDown size={12} />
        </button>
      </div>

      {open && (
        <div className="absolute right-0 top-full mt-1 bg-zinc-900 border border-zinc-800 rounded-md shadow-lg z-10 min-w-[120px]">
          <button
            onClick={() => { setLevel('sessions'); setOpen(false) }}
            className={`block w-full text-left px-3 py-1.5 text-sm transition-colors ${level === 'sessions' ? 'text-zinc-200 bg-zinc-800' : 'text-zinc-400 hover:bg-zinc-800 hover:text-zinc-300'}`}
          >
            {t.sessions_export_sessions}
          </button>
          <button
            onClick={() => { setLevel('spans'); setOpen(false) }}
            className={`block w-full text-left px-3 py-1.5 text-sm transition-colors ${level === 'spans' ? 'text-zinc-200 bg-zinc-800' : 'text-zinc-400 hover:bg-zinc-800 hover:text-zinc-300'}`}
          >
            {t.sessions_export_spans}
          </button>
        </div>
      )}
    </div>
  )
}
