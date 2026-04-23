import { useState, useMemo } from 'react'
import { useAuthStore } from '@/store'
import { useI18n } from '@/i18n'
import { useStats, useDailyStats, useAgentStats, useFinishReasons } from '@/hooks/use-dashboard'
import { useSessionsSocket } from '@/hooks/use-websocket'
import { KPICard } from '@/components/app/KPICard'
import { DailyChart } from '@/components/app/DailyChart'
import { DateRangePicker } from '@/components/app/DateRangePicker'
import { OnboardingChecklist } from '@/components/app/OnboardingChecklist'
import { EmptyState } from '@/components/app/EmptyState'
import { formatCost, formatDuration, formatNumber, getDateRange } from '@/lib/date'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { Info } from 'lucide-react'
import type { DateRange } from '@/lib/date'

export function DashboardPage() {
  const { t, tt } = useI18n()
  const [dateRange, setDateRange] = useState<DateRange>('7d')
  const activeOrgID = useAuthStore((s) => s.activeOrgID) ?? ''
  const { from, to } = useMemo(() => getDateRange(dateRange), [dateRange])

  useSessionsSocket(activeOrgID)

  const statsQuery = useStats(activeOrgID, from, to)
  const dailyQuery = useDailyStats(activeOrgID, dateRange)
  const agentQuery = useAgentStats(activeOrgID, from, to)
  const finishQuery = useFinishReasons(activeOrgID, from, to)

  const isLoading = statsQuery.isLoading || dailyQuery.isLoading
  const hasError = statsQuery.isError || dailyQuery.isError
  const totalSessions = statsQuery.data?.total_sessions ?? 0
  const totalSpans = statsQuery.data?.total_spans ?? 0
  const hasActivity = totalSessions > 0 || totalSpans > 0

  return (
    <div className="p-6 lg:p-8 space-y-8 animate-fade-in-up">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-zinc-50 tracking-[-0.01em]">{t.dash_title}</h1>
          <p className="text-sm text-zinc-500 mt-1">{t.dash_subtitle}</p>
        </div>
        <DateRangePicker value={dateRange} onChange={setDateRange} />
      </div>

      {hasError && (
        <p className="text-sm text-zinc-500">
          {t.dash_failed_stats}{' '}
          <button onClick={() => { statsQuery.refetch(); dailyQuery.refetch() }} className="text-zinc-300 hover:text-zinc-50 transition-colors">{t.common_retry}</button>
        </p>
      )}

      {/* KPI cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 lg:grid-cols-5 gap-3">
        {isLoading ? (
          Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
              <div className="h-3 skeleton-shimmer rounded w-20 mb-3" />
              <div className="h-7 skeleton-shimmer rounded w-16" />
            </div>
          ))
        ) : statsQuery.data ? (
          <>
            <KPICard title={t.dash_sessions} value={formatNumber(statsQuery.data.total_sessions)} className="animate-fade-in-up" />
            <KPICard title={t.dash_spans} value={formatNumber(statsQuery.data.total_spans)} className="animate-fade-in-up stagger-1" />
            <KPICard title={t.dash_total_cost} value={formatCost(statsQuery.data.total_cost_usd)} className="animate-fade-in-up stagger-2" />
            <KPICard title={t.dash_avg_latency} value={formatDuration(statsQuery.data.avg_duration_ms)} className="animate-fade-in-up stagger-3" />
            <KPICard title={t.dash_error_rate} value={`${(statsQuery.data.error_rate * 100).toFixed(1)}%`} alert={statsQuery.data.error_rate > 0.05} className="animate-fade-in-up stagger-4" />
          </>
        ) : null}
      </div>

      {/* Chart */}
      {isLoading ? (
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-5">
          <div className="h-4 skeleton-shimmer rounded w-32 mb-5" />
          <div className="h-[260px] skeleton-shimmer rounded" />
        </div>
      ) : dailyQuery.data ? (
        <div className="animate-fade-in-up stagger-4">
          <DailyChart data={dailyQuery.data} />
        </div>
      ) : null}

      {/* Finish Reasons */}
      {!isLoading && finishQuery.isError && (
        <p className="text-sm text-zinc-500">{t.dash_failed_finish_reasons} <button onClick={() => finishQuery.refetch()} className="text-zinc-300 hover:text-zinc-50 transition-colors">{t.common_retry}</button></p>
      )}
      {!isLoading && finishQuery.data && finishQuery.data.length > 0 && (
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-5 animate-fade-in-up stagger-4">
          <div className="flex items-center gap-2 mb-4">
            <h3 className="text-xs font-medium text-zinc-500 uppercase tracking-wider">{t.dash_finish_reasons}</h3>
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button type="button" className="text-zinc-600 hover:text-zinc-400 transition-colors">
                    <Info size={13} />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="right">
                  <ul className="space-y-1.5 text-[11px] leading-relaxed">
                    <li><span className="text-emerald-400 font-medium">{t.dash_finish_stop_label}</span> — {t.dash_finish_stop}</li>
                    <li><span className="text-amber-400 font-medium">{t.dash_finish_length_label}</span> — {t.dash_finish_length}</li>
                    <li><span className="text-red-400 font-medium">{t.dash_finish_content_filter_label}</span> — {t.dash_finish_content_filter}</li>
                    <li><span className="text-blue-400 font-medium">{t.dash_finish_tool_calls_label}</span> — {t.dash_finish_tool_calls}</li>
                    <li><span className="text-zinc-500 font-medium">{t.dash_finish_null_label}</span> — {t.dash_finish_null}</li>
                  </ul>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
          <div className="flex flex-wrap gap-3">
            {finishQuery.data.map((item) => {
              const total = finishQuery.data!.reduce((sum, r) => sum + r.count, 0)
              const pct = total > 0 ? (item.count / total * 100).toFixed(1) : '0'
              const color = item.finish_reason === 'stop' || item.finish_reason === 'end_turn'
                ? 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20'
                : item.finish_reason === 'length'
                ? 'text-amber-400 bg-amber-400/10 border-amber-400/20'
                : item.finish_reason === 'content_filter'
                ? 'text-red-400 bg-red-400/10 border-red-400/20'
                : item.finish_reason === 'tool_calls' || item.finish_reason === 'tool_use'
                ? 'text-blue-400 bg-blue-400/10 border-blue-400/20'
                : 'text-zinc-400 bg-zinc-400/10 border-zinc-400/20'
              return (
                <div key={item.finish_reason} className={`px-3 py-2 rounded-lg border text-sm ${color}`}>
                  <span className="font-medium">{item.finish_reason}</span>
                  <span className="ml-2 font-mono tabular-nums text-xs opacity-80">{formatNumber(item.count)}</span>
                  <span className="ml-1 text-xs opacity-60">({pct}%)</span>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {/* Agent stats */}
      {!isLoading && agentQuery.isError && (
        <p className="text-sm text-zinc-500">{t.dash_failed_agents} <button onClick={() => agentQuery.refetch()} className="text-zinc-300 hover:text-zinc-50 transition-colors">{t.common_retry}</button></p>
      )}
      {!isLoading && agentQuery.data && agentQuery.data.length > 0 && (
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg overflow-hidden animate-fade-in-up stagger-4">
          <div className="px-5 pt-5 pb-3 flex items-center justify-between">
            <h3 className="text-xs font-medium text-zinc-500 uppercase tracking-wider">{t.dash_agents}</h3>
            <span className="text-xs text-zinc-600 tabular-nums">{tt('dash_agents_active', { count: agentQuery.data.length })}</span>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-sm text-left">
              <thead>
                <tr className="border-t border-b border-zinc-800/60">
                  <th className="px-5 py-2.5 text-xs font-medium text-zinc-600">{t.dash_agent_name}</th>
                  <th className="px-5 py-2.5 text-xs font-medium text-zinc-600 text-right">{t.dash_agent_sessions}</th>
                  <th className="px-5 py-2.5 text-xs font-medium text-zinc-600 text-right">{t.dash_agent_spans}</th>
                  <th className="px-5 py-2.5 text-xs font-medium text-zinc-600 text-right">{t.dash_agent_cost}</th>
                  <th className="px-5 py-2.5 text-xs font-medium text-zinc-600 text-right">{t.dash_agent_latency}</th>
                  <th className="px-5 py-2.5 text-xs font-medium text-zinc-600 text-right">{t.dash_agent_token_ratio}</th>
                  <th className="px-5 py-2.5 text-xs font-medium text-zinc-600 text-right">{t.dash_agent_error_rate}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-800/40">
                {agentQuery.data.map((agent) => (
                  <tr key={agent.api_key_id} className="table-row-hover">
                    <td className="px-5 py-3 text-zinc-200 font-medium">{agent.api_key_name}</td>
                    <td className="px-5 py-3 text-zinc-400 text-right font-mono tabular-nums">{formatNumber(agent.session_count)}</td>
                    <td className="px-5 py-3 text-zinc-400 text-right font-mono tabular-nums">{formatNumber(agent.span_count)}</td>
                    <td className="px-5 py-3 text-zinc-400 text-right font-mono tabular-nums">{formatCost(agent.total_cost_usd)}</td>
                    <td className="px-5 py-3 text-zinc-400 text-right font-mono tabular-nums">{formatDuration(agent.avg_duration_ms)}</td>
                    <td className={`px-5 py-3 text-right font-mono tabular-nums ${agent.avg_token_ratio < 0.05 || agent.avg_token_ratio > 20 ? 'text-amber-400' : 'text-zinc-400'}`}>{agent.avg_token_ratio > 0 ? agent.avg_token_ratio.toFixed(2) : '--'}</td>
                    <td className={`px-5 py-3 text-right font-mono tabular-nums ${agent.error_rate > 0.05 ? 'text-red-400' : 'text-zinc-400'}`}>{(agent.error_rate * 100).toFixed(1)}%</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {!isLoading && <OnboardingChecklist hasSessions={hasActivity} orgID={activeOrgID} />}

      {!isLoading && !hasActivity && (
        <div className="animate-fade-in-up">
          <EmptyState
            heading={t.dash_no_activity_title}
            body={t.dash_no_activity_body}
            action={{ label: t.dash_create_key, href: '/keys' }}
          />
        </div>
      )}
    </div>
  )
}
