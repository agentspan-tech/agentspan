import { useParams, Link } from 'react-router-dom'
import { useAuthStore } from '@/store'
import { useI18n } from '@/i18n'
import { useSystemPrompt } from '@/hooks/use-system-prompts'
import { formatTimestamp, formatRelative, formatNumber } from '@/lib/date'
import { ArrowLeft, Copy, Check } from 'lucide-react'
import { useState } from 'react'

export function SystemPromptDetailPage() {
  const { t } = useI18n()
  const { id } = useParams<{ id: string }>()
  const activeOrgID = useAuthStore((s) => s.activeOrgID) ?? ''
  const { data: prompt, isLoading, isError } = useSystemPrompt(activeOrgID, id ?? null)
  const [copied, setCopied] = useState(false)

  function copyContent() {
    if (!prompt) return
    navigator.clipboard.writeText(prompt.content).catch(() => {})
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="p-6 lg:p-8 space-y-6 animate-fade-in-up">
      <Link to="/system-prompts" className="flex items-center gap-2 text-sm text-zinc-500 hover:text-zinc-200 transition-colors w-fit">
        <ArrowLeft size={14} /> {t.prompts_back}
      </Link>

      {isLoading ? (
        <div className="space-y-4">
          <div className="h-6 skeleton-shimmer rounded w-32" />
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            {[1, 2, 3, 4].map((i) => <div key={i} className="h-16 skeleton-shimmer rounded-lg" />)}
          </div>
          <div className="h-64 skeleton-shimmer rounded-lg" />
        </div>
      ) : isError || !prompt ? (
        <p className="text-sm text-zinc-500">{t.prompts_not_found}</p>
      ) : (
        <>
          <h1 className="text-xl font-semibold text-zinc-50 font-mono tracking-[-0.01em]">{prompt.short_uid}</h1>

          {/* Stats */}
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
              <p className="text-xs text-zinc-500 uppercase tracking-wider font-medium mb-1.5">{t.prompts_detail_spans}</p>
              <p className="text-lg font-semibold text-zinc-50 tabular-nums">{formatNumber(prompt.span_count)}</p>
            </div>
            <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
              <p className="text-xs text-zinc-500 uppercase tracking-wider font-medium mb-1.5">{t.prompts_detail_sessions}</p>
              <p className="text-lg font-semibold text-zinc-50 tabular-nums">{formatNumber(prompt.session_count)}</p>
            </div>
            <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
              <p className="text-xs text-zinc-500 uppercase tracking-wider font-medium mb-1.5">{t.prompts_detail_created}</p>
              <p className="text-sm text-zinc-200">{formatTimestamp(prompt.created_at)}</p>
            </div>
            <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
              <p className="text-xs text-zinc-500 uppercase tracking-wider font-medium mb-1.5">{t.prompts_detail_last_seen}</p>
              <p className="text-sm text-zinc-200">{prompt.last_seen_at ? formatRelative(prompt.last_seen_at) : '--'}</p>
            </div>
          </div>

          {/* Content */}
          <div className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden">
            <div className="flex items-center justify-between px-5 pt-4 pb-2">
              <h3 className="text-xs font-medium text-zinc-500 uppercase tracking-wider">{t.prompts_content}</h3>
              <button
                onClick={copyContent}
                className="flex items-center gap-1.5 text-xs text-zinc-500 hover:text-zinc-200 transition-colors"
              >
                {copied ? <><Check size={12} className="text-emerald-400" /> {t.common_copied}</> : <><Copy size={12} /> {t.common_copy}</>}
              </button>
            </div>
            <pre className="px-5 pb-5 pt-2 text-sm text-zinc-400 font-mono overflow-x-auto max-h-[70vh] overflow-y-auto whitespace-pre-wrap break-words leading-relaxed">
              {prompt.content}
            </pre>
          </div>
        </>
      )}
    </div>
  )
}
