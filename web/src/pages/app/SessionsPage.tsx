import { useSearchParams, useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/store'
import { useI18n } from '@/i18n'
import { useSessions } from '@/hooks/use-sessions'
import { useSessionsSocket } from '@/hooks/use-websocket'
import { SessionStatusBadge } from '@/components/app/SessionStatusBadge'
import { EmptyState } from '@/components/app/EmptyState'
import { formatCost, formatRelative } from '@/lib/date'
import type { SessionStatus } from '@/types/api'
import { Loader2, Search } from 'lucide-react'

export function SessionsPage() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const activeOrgID = useAuthStore((s) => s.activeOrgID) ?? ''

  const status = searchParams.get('status') ?? ''
  const agentName = searchParams.get('agent_name') ?? ''
  const apiKeyID = searchParams.get('api_key_id') ?? ''
  const providerType = searchParams.get('provider_type') ?? ''

  const filters = {
    status: status || undefined,
    agent_name: agentName || undefined,
    api_key_id: apiKeyID || undefined,
    provider_type: providerType || undefined,
  }

  useSessionsSocket(activeOrgID)

  const { data, isLoading, isFetchingNextPage, hasNextPage, fetchNextPage } =
    useSessions(activeOrgID, filters)

  const sessions = data?.pages.flatMap((p) => p.data) ?? []

  function setParam(key: string, value: string) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value) next.set(key, value)
      else next.delete(key)
      return next
    })
  }

  function clearFilters() { setSearchParams({}) }

  const hasFilters = !!(status || agentName || apiKeyID || providerType)

  const SESSION_STATUSES = [
    { label: t.sessions_status_all, value: '' },
    { label: t.sessions_status_in_progress, value: 'in_progress' },
    { label: t.sessions_status_completed, value: 'completed' },
    { label: t.sessions_status_with_errors, value: 'completed_with_errors' },
    { label: t.sessions_status_failed, value: 'failed' },
    { label: t.sessions_status_abandoned, value: 'abandoned' },
  ]

  const PROVIDER_TYPES = [
    { label: t.sessions_provider_all, value: '' },
    { label: t.sessions_provider_openai, value: 'openai' },
    { label: t.sessions_provider_anthropic, value: 'anthropic' },
    { label: t.sessions_provider_deepseek, value: 'deepseek' },
    { label: t.sessions_provider_mistral, value: 'mistral' },
    { label: t.sessions_provider_groq, value: 'groq' },
    { label: t.sessions_provider_gemini, value: 'gemini' },
    { label: t.sessions_provider_custom, value: 'custom' },
  ]

  return (
    <div className="p-6 lg:p-8 space-y-6 animate-fade-in-up">
      <div>
        <h1 className="text-xl font-semibold text-zinc-50 tracking-[-0.01em]">{t.sessions_title}</h1>
        <p className="text-sm text-zinc-500 mt-1">{t.sessions_subtitle}</p>
      </div>

      {/* Filters */}
      <div className="flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-3">
        <div className="flex items-center gap-2 sm:gap-3">
          <select
            value={status}
            onChange={(e) => setParam('status', e.target.value)}
            className="bg-zinc-900 border border-zinc-800 rounded-md px-3 py-1.5 text-sm text-zinc-300 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 flex-1 sm:flex-none transition-colors"
          >
            {SESSION_STATUSES.map((s) => (
              <option key={s.value} value={s.value}>{s.label}</option>
            ))}
          </select>

          <select
            value={providerType}
            onChange={(e) => setParam('provider_type', e.target.value)}
            className="bg-zinc-900 border border-zinc-800 rounded-md px-3 py-1.5 text-sm text-zinc-300 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 flex-1 sm:flex-none transition-colors"
          >
            {PROVIDER_TYPES.map((p) => (
              <option key={p.value} value={p.value}>{p.label}</option>
            ))}
          </select>
        </div>

        <div className="flex items-center gap-2 sm:gap-3">
          <div className="relative flex-1 sm:flex-none">
            <Search size={13} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-600" />
            <input
              value={agentName}
              onChange={(e) => setParam('agent_name', e.target.value)}
              placeholder={t.sessions_agent_placeholder}
              className="bg-zinc-900 border border-zinc-800 rounded-md pl-8 pr-3 py-1.5 text-sm text-zinc-300 placeholder:text-zinc-600 focus:outline-none focus:border-zinc-600 focus:ring-1 focus:ring-zinc-600 w-full sm:w-44 transition-colors"
            />
          </div>

          {hasFilters && (
            <button onClick={clearFilters} className="text-sm text-zinc-500 hover:text-zinc-300 transition-colors whitespace-nowrap">
              {t.sessions_clear_filters}
            </button>
          )}
        </div>
      </div>

      {/* Table */}
      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-12 skeleton-shimmer rounded-lg" />
          ))}
        </div>
      ) : sessions.length === 0 ? (
        <EmptyState
          heading={hasFilters ? t.sessions_no_matches : t.sessions_no_sessions}
          body={hasFilters ? t.sessions_no_matches_body : t.sessions_no_sessions_body}
          action={hasFilters ? { label: t.sessions_clear_filters, onClick: clearFilters } : { label: t.dash_create_key, href: '/keys' }}
        />
      ) : (
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden animate-fade-in">
          <div className="overflow-x-auto">
            <table className="w-full text-sm text-left">
              <thead>
                <tr className="border-b border-zinc-800/60">
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.sessions_col_status}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.sessions_col_agent}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden md:table-cell">{t.sessions_col_key}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.sessions_col_spans}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.sessions_col_cost}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.sessions_col_started}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden lg:table-cell">{t.sessions_col_activity}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-800/40">
                {sessions.map((session, i) => (
                  <tr
                    key={session.id}
                    className="table-row-hover cursor-pointer animate-fade-in-up"
                    style={{ animationDelay: `${Math.min(i * 30, 300)}ms` }}
                    onClick={() => navigate(`/sessions/${session.id}`)}
                  >
                    <td className="px-5 py-3.5"><SessionStatusBadge status={session.status as SessionStatus} /></td>
                    <td className="px-5 py-3.5 text-zinc-200 font-medium">{session.agent_name ?? session.api_key_name}</td>
                    <td className="px-5 py-3.5 text-zinc-500 hidden md:table-cell">{session.api_key_name}</td>
                    <td className="px-5 py-3.5 text-zinc-400 tabular-nums">{session.span_count}</td>
                    <td className="px-5 py-3.5 text-zinc-200 font-mono tabular-nums">{formatCost(session.total_cost_usd)}</td>
                    <td className="px-5 py-3.5 text-zinc-500">{formatRelative(session.started_at)}</td>
                    <td className="px-5 py-3.5 text-zinc-600 hidden lg:table-cell">{formatRelative(session.last_span_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {hasNextPage && (
        <button
          onClick={() => fetchNextPage()}
          disabled={isFetchingNextPage}
          className="w-full text-center text-sm font-medium bg-zinc-900 border border-zinc-800 text-zinc-300 px-5 py-2.5 rounded-md hover:bg-zinc-800 hover:border-zinc-700 transition-colors duration-150 flex items-center justify-center gap-2 disabled:opacity-50 btn-press"
        >
          {isFetchingNextPage ? <><Loader2 size={14} className="animate-spin" /> {t.sessions_loading}</> : t.sessions_load_more}
        </button>
      )}
    </div>
  )
}
