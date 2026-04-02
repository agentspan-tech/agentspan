import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/store'
import { useI18n } from '@/i18n'
import { useSystemPrompts } from '@/hooks/use-system-prompts'
import { EmptyState } from '@/components/app/EmptyState'
import { formatRelative, formatNumber } from '@/lib/date'
import { FileText } from 'lucide-react'

export function SystemPromptsPage() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const activeOrgID = useAuthStore((s) => s.activeOrgID) ?? ''
  const { data: prompts, isLoading, isError, refetch } = useSystemPrompts(activeOrgID)

  return (
    <div className="p-6 lg:p-8 space-y-6 animate-fade-in-up">
      <div>
        <h1 className="text-xl font-semibold text-zinc-50 tracking-[-0.01em]">{t.prompts_title}</h1>
        <p className="text-sm text-zinc-500 mt-1">{t.prompts_subtitle}</p>
      </div>

      {isError && (
        <p className="text-sm text-zinc-500">
          {t.prompts_failed}{' '}
          <button onClick={() => refetch()} className="text-zinc-300 hover:text-zinc-50 transition-colors">{t.common_retry}</button>
        </p>
      )}

      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-16 skeleton-shimmer rounded-lg" />
          ))}
        </div>
      ) : !prompts || prompts.length === 0 ? (
        <EmptyState
          heading={t.prompts_empty_title}
          body={t.prompts_empty_body}
        />
      ) : (
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden animate-fade-in">
          <div className="overflow-x-auto">
            <table className="w-full text-sm text-left">
              <thead>
                <tr className="border-b border-zinc-800/60">
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.prompts_col_prompt}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider text-right">{t.prompts_col_spans}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider text-right">{t.prompts_col_sessions}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider text-right hidden md:table-cell">{t.prompts_col_length}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden lg:table-cell">{t.prompts_col_last_seen}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden lg:table-cell">{t.prompts_col_created}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-800/40">
                {prompts.map((prompt, i) => (
                  <tr
                    key={prompt.id}
                    className="table-row-hover cursor-pointer animate-fade-in-up"
                    style={{ animationDelay: `${Math.min(i * 30, 300)}ms` }}
                    onClick={() => navigate(`/system-prompts/${prompt.id}`)}
                  >
                    <td className="px-5 py-3.5 max-w-md">
                      <div className="flex items-start gap-2.5">
                        <FileText size={14} className="text-zinc-700 mt-0.5 shrink-0" />
                        <div className="min-w-0">
                          <span className="text-[11px] font-mono text-zinc-600 tabular-nums">{prompt.short_uid}</span>
                          <p className="text-sm text-zinc-300 truncate">{prompt.content_preview}</p>
                        </div>
                      </div>
                    </td>
                    <td className="px-5 py-3.5 text-zinc-400 text-right font-mono tabular-nums">{formatNumber(prompt.span_count)}</td>
                    <td className="px-5 py-3.5 text-zinc-400 text-right font-mono tabular-nums">{formatNumber(prompt.session_count)}</td>
                    <td className="px-5 py-3.5 text-zinc-500 text-right font-mono tabular-nums hidden md:table-cell">{formatNumber(prompt.content_length)} {t.prompts_chars}</td>
                    <td className="px-5 py-3.5 text-zinc-500 hidden lg:table-cell">{prompt.last_seen_at ? formatRelative(prompt.last_seen_at) : '--'}</td>
                    <td className="px-5 py-3.5 text-zinc-600 hidden lg:table-cell">{formatRelative(prompt.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
