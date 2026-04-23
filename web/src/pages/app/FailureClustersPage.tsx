import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuthStore } from '@/store'
import { useI18n } from '@/i18n'
import { useFailureClusters, useClusterSessions } from '@/hooks/use-failure-clusters'
import { SessionStatusBadge } from '@/components/app/SessionStatusBadge'
import { EmptyState } from '@/components/app/EmptyState'
import { formatRelative } from '@/lib/date'
import { ArrowLeft, AlertTriangle } from 'lucide-react'
import type { SessionStatus } from '@/types/api'

const CATEGORY_LABELS: Record<string, { en: string; color: string }> = {
  hallucination: { en: 'Hallucination', color: 'text-red-400 bg-red-500/10' },
  context_loss: { en: 'Context Loss', color: 'text-orange-400 bg-orange-500/10' },
  echo: { en: 'Echo / Parroting', color: 'text-amber-400 bg-amber-500/10' },
  off_topic: { en: 'Off Topic', color: 'text-purple-400 bg-purple-500/10' },
  empty_output: { en: 'Empty Output', color: 'text-zinc-400 bg-zinc-500/10' },
  malformed: { en: 'Malformed', color: 'text-yellow-400 bg-yellow-500/10' },
}

function categoryBadge(label: string) {
  const cat = CATEGORY_LABELS[label]
  if (cat) {
    return <span className={`text-[11px] font-medium px-2 py-0.5 rounded ${cat.color}`}>{cat.en}</span>
  }
  return <span className="text-[11px] font-medium px-2 py-0.5 rounded text-zinc-400 bg-zinc-500/10">{label}</span>
}

export function FailureClustersPage() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const activeOrgID = useAuthStore((s) => s.activeOrgID) ?? ''
  const { data: clusters, isLoading, isError, refetch } = useFailureClusters(activeOrgID)
  const [selectedClusterID, setSelectedClusterID] = useState<string | null>(null)
  const { data: sessions, isLoading: sessionsLoading } = useClusterSessions(activeOrgID, selectedClusterID)

  const selectedCluster = clusters?.find((c) => c.id === selectedClusterID)

  if (selectedClusterID && selectedCluster) {
    return (
      <div className="p-6 lg:p-8 space-y-6 animate-fade-in-up">
        <button
          onClick={() => setSelectedClusterID(null)}
          className="flex items-center gap-2 text-sm text-zinc-500 hover:text-zinc-200 transition-colors w-fit"
        >
          <ArrowLeft size={14} /> {t.clusters_back}
        </button>

        <div className="space-y-2">
          <div className="flex items-center gap-3">
            {categoryBadge(selectedCluster.label)}
          </div>
          <p className="text-sm text-zinc-500">
            {selectedCluster.session_count} {t.clusters_col_sessions.toLowerCase()}
          </p>
        </div>

        <div>
          <h3 className="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-4">{t.clusters_sessions_title}</h3>
          {sessionsLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="h-14 skeleton-shimmer rounded-lg" />
              ))}
            </div>
          ) : !sessions || sessions.length === 0 ? (
            <p className="text-sm text-zinc-600 py-8 text-center">No sessions found.</p>
          ) : (
            <div className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden">
              <div className="overflow-x-auto">
                <table className="w-full text-sm text-left">
                  <thead>
                    <tr className="border-b border-zinc-800/60">
                      <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">Agent</th>
                      <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">Status</th>
                      <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider text-right">Spans</th>
                      <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden md:table-cell">Started</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-zinc-800/40">
                    {sessions.map((s, i) => (
                      <tr
                        key={s.id}
                        className="table-row-hover cursor-pointer animate-fade-in-up"
                        style={{ animationDelay: `${Math.min(i * 30, 300)}ms` }}
                        onClick={() => navigate(`/sessions/${s.id}`)}
                      >
                        <td className="px-5 py-3.5">
                          <span className="text-sm text-zinc-300">{s.agent_name ?? s.api_key_name}</span>
                        </td>
                        <td className="px-5 py-3.5">
                          <SessionStatusBadge status={s.status as SessionStatus} />
                        </td>
                        <td className="px-5 py-3.5 text-zinc-400 text-right font-mono tabular-nums">{s.span_count}</td>
                        <td className="px-5 py-3.5 text-zinc-500 hidden md:table-cell">{formatRelative(s.started_at)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="p-6 lg:p-8 space-y-6 animate-fade-in-up">
      <div>
        <h1 className="text-xl font-semibold text-zinc-50 tracking-[-0.01em]">{t.clusters_title}</h1>
        <p className="text-sm text-zinc-500 mt-1">{t.clusters_subtitle}</p>
      </div>

      {isError && (
        <p className="text-sm text-zinc-500">
          {t.clusters_failed}{' '}
          <button onClick={() => refetch()} className="text-zinc-300 hover:text-zinc-50 transition-colors">{t.common_retry}</button>
        </p>
      )}

      {isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-16 skeleton-shimmer rounded-lg" />
          ))}
        </div>
      ) : !clusters || clusters.length === 0 ? (
        <EmptyState
          heading={t.clusters_empty_title}
          body={t.clusters_empty_body}
        />
      ) : (
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden animate-fade-in">
          <div className="overflow-x-auto">
            <table className="w-full text-sm text-left">
              <thead>
                <tr className="border-b border-zinc-800/60">
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider">{t.clusters_col_pattern}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider text-right">{t.clusters_col_sessions}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden md:table-cell">{t.clusters_col_last_seen}</th>
                  <th className="px-5 py-3 text-xs font-medium text-zinc-600 uppercase tracking-wider hidden lg:table-cell">{t.clusters_col_created}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-800/40">
                {clusters.map((cluster, i) => (
                  <tr
                    key={cluster.id}
                    className="table-row-hover cursor-pointer animate-fade-in-up"
                    style={{ animationDelay: `${Math.min(i * 30, 300)}ms` }}
                    onClick={() => setSelectedClusterID(cluster.id)}
                  >
                    <td className="px-5 py-3.5">
                      <div className="flex items-center gap-2.5">
                        <AlertTriangle size={14} className="text-orange-500/60 shrink-0" />
                        {categoryBadge(cluster.label)}
                      </div>
                    </td>
                    <td className="px-5 py-3.5 text-zinc-400 text-right font-mono tabular-nums">{cluster.session_count}</td>
                    <td className="px-5 py-3.5 text-zinc-500 hidden md:table-cell">{formatRelative(cluster.updated_at)}</td>
                    <td className="px-5 py-3.5 text-zinc-600 hidden lg:table-cell">{formatRelative(cluster.created_at)}</td>
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
