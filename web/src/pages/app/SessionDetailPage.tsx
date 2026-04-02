import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useAuthStore } from '@/store'
import { useI18n } from '@/i18n'
import { useSession } from '@/hooks/use-sessions'
import { useSessionSocket } from '@/hooks/use-websocket'
import { SessionStatusBadge } from '@/components/app/SessionStatusBadge'
import { formatCost, formatDuration, formatTimestamp } from '@/lib/date'
import { ArrowLeft, ChevronRight } from 'lucide-react'
import type { SessionStatus, SpanItem } from '@/types/api'

function httpStatusColor(status: number): string {
  if (status >= 200 && status < 300) return 'text-emerald-400'
  if (status >= 400 && status < 500) return 'text-amber-400'
  if (status >= 500) return 'text-red-400'
  return 'text-zinc-500'
}

function httpStatusDot(status: number): string {
  if (status >= 200 && status < 300) return 'bg-emerald-400'
  if (status >= 400 && status < 500) return 'bg-amber-400'
  if (status >= 500) return 'bg-red-400'
  return 'bg-zinc-600'
}

function finishReasonBadge(reason?: string) {
  if (!reason) return null
  const styles: Record<string, string> = {
    stop: 'text-emerald-400 bg-emerald-400/10',
    end_turn: 'text-emerald-400 bg-emerald-400/10',
    length: 'text-amber-400 bg-amber-400/10',
    content_filter: 'text-red-400 bg-red-400/10',
    tool_calls: 'text-blue-400 bg-blue-400/10',
    tool_use: 'text-blue-400 bg-blue-400/10',
  }
  const style = styles[reason] ?? 'text-zinc-500 bg-zinc-500/10'
  return <span className={`text-[10px] font-medium px-1.5 py-0.5 rounded ${style}`}>{reason}</span>
}

function formatGap(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60000).toFixed(1)}m`
}

/** Assign each span to a lane so overlapping spans stack vertically. */
function assignLanes(spans: SpanItem[]): { lane: number; laneCount: number }[] {
  // Each lane tracks its "end time" (the earliest time it becomes free).
  const laneEnds: number[] = []
  const assignments: { lane: number; laneCount: number }[] = []

  for (const span of spans) {
    const spanStart = new Date(span.started_at).getTime()
    const spanEnd = spanStart + span.duration_ms

    // Find first lane where the span fits (lane is free before this span starts).
    let assigned = -1
    for (let l = 0; l < laneEnds.length; l++) {
      if (laneEnds[l] <= spanStart) {
        assigned = l
        laneEnds[l] = spanEnd
        break
      }
    }
    if (assigned === -1) {
      assigned = laneEnds.length
      laneEnds.push(spanEnd)
    }
    assignments.push({ lane: assigned, laneCount: 0 })
  }

  // Fill laneCount for all entries.
  const totalLanes = laneEnds.length
  for (const a of assignments) a.laneCount = totalLanes
  return assignments
}

function spanColor(span: SpanItem): string {
  if (span.http_status >= 500) return 'bg-red-500/80'
  if (span.http_status >= 400) return 'bg-amber-500/80'
  if (span.finish_reason === 'length') return 'bg-amber-400/60'
  if (span.finish_reason === 'content_filter') return 'bg-red-400/60'
  if (span.finish_reason === 'tool_calls' || span.finish_reason === 'tool_use') return 'bg-blue-400/60'
  return 'bg-emerald-500/70'
}

export function SessionDetailPage() {
  const { t, tt } = useI18n()
  const { id } = useParams<{ id: string }>()
  const activeOrgID = useAuthStore((s) => s.activeOrgID) ?? ''
  const { data: session, isLoading, isError } = useSession(activeOrgID, id ?? '')

  useSessionSocket(id ?? '')

  // Compute inter-span gaps
  const spansWithGaps = session?.spans.map((span, i) => {
    if (i === 0) return { ...span, gapMs: 0 }
    const prev = session.spans[i - 1]
    const gapMs = new Date(span.started_at).getTime() - new Date(prev.started_at).getTime() - prev.duration_ms
    return { ...span, gapMs: Math.max(0, gapMs) }
  }) ?? []

  // Timeline bar data with lane assignments for parallel spans.
  const timelineData = session?.spans && session.spans.length > 1 ? (() => {
    const start = new Date(session.spans[0].started_at).getTime()
    // Find the true end: max(start + duration) across all spans.
    let end = 0
    for (const sp of session.spans) {
      const spEnd = new Date(sp.started_at).getTime() + sp.duration_ms
      if (spEnd > end) end = spEnd
    }
    const totalMs = end - start
    if (totalMs <= 0) return null
    const lanes = assignLanes(session.spans)
    return { start, totalMs, lanes }
  })() : null

  return (
    <div className="p-6 lg:p-8 space-y-6 animate-fade-in-up">
      <Link to="/sessions" className="flex items-center gap-2 text-sm text-zinc-500 hover:text-zinc-200 transition-colors w-fit">
        <ArrowLeft size={14} /> {t.detail_back}
      </Link>

      {isLoading ? (
        <div className="space-y-4">
          <div className="h-6 skeleton-shimmer rounded w-48" />
          <div className="h-24 skeleton-shimmer rounded-lg" />
          <div className="space-y-3">
            {[1, 2, 3].map((i) => <div key={i} className="h-14 skeleton-shimmer rounded-lg" />)}
          </div>
        </div>
      ) : isError ? (
        <p className="text-sm text-zinc-500">{t.detail_not_found}</p>
      ) : session ? (
        <>
          {/* Header */}
          <div className="space-y-3">
            <div className="flex items-center gap-3">
              <SessionStatusBadge status={session.status as SessionStatus} />
              <h1 className="text-xl font-semibold text-zinc-50 tracking-[-0.01em]">{session.agent_name ?? session.api_key_name}</h1>
            </div>
            <div className="flex flex-wrap gap-5 text-sm text-zinc-500">
              <span>{tt('detail_spans_label', { count: session.span_count })}</span>
              <span>{t.detail_cost_label} <span className="text-zinc-200 font-mono tabular-nums">{formatCost(session.total_cost_usd)}</span></span>
              {session.spans.length > 0 && <span>{t.detail_model_label} <span className="text-zinc-200">{session.spans[0].model}</span></span>}
              {session.closed_at && session.started_at && (
                <span>{t.detail_duration_label} <span className="text-zinc-200">{formatDuration(new Date(session.closed_at).getTime() - new Date(session.started_at).getTime())}</span></span>
              )}
            </div>
          </div>

          {/* Narrative */}
          <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-5">
            <h3 className="text-xs font-medium text-zinc-500 uppercase tracking-wider mb-3">{t.detail_analysis}</h3>
            {session.narrative ? (
              <p className="text-sm text-zinc-400 leading-relaxed">{session.narrative}</p>
            ) : (
              <p className="text-sm text-zinc-600 italic">{t.detail_analysis_pending}</p>
            )}
          </div>

          {/* Session Timeline — multi-lane waterfall for parallel spans */}
          {timelineData && session.spans.length > 1 && (() => {
            const laneCount = timelineData.lanes[0].laneCount
            // Generate time tick marks (4-6 ticks evenly spaced).
            const tickCount = 5
            const ticks = Array.from({ length: tickCount + 1 }, (_, i) => ({
              pct: (i / tickCount) * 100,
              ms: Math.round((i / tickCount) * timelineData.totalMs),
            }))
            return (
              <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-5">
                <div className="flex items-center justify-between mb-3">
                  <h3 className="text-xs font-medium text-zinc-500 uppercase tracking-wider">{t.detail_timeline}</h3>
                  <div className="flex items-center gap-3">
                    {laneCount > 1 && (
                      <span className="text-[10px] text-zinc-600">{tt('detail_parallel_lanes', { count: laneCount })}</span>
                    )}
                    <span className="text-[10px] text-zinc-600 font-mono tabular-nums">{tt('detail_total_duration', { duration: formatDuration(timelineData.totalMs) })}</span>
                  </div>
                </div>
                {/* Tick marks row */}
                <div className="relative h-4 mb-0.5">
                  {ticks.map((tick) => (
                    <div key={tick.pct} className="absolute flex flex-col items-center" style={{ left: `${tick.pct}%`, transform: 'translateX(-50%)' }}>
                      <span className="text-[9px] text-zinc-600 font-mono tabular-nums whitespace-nowrap">{formatDuration(tick.ms)}</span>
                    </div>
                  ))}
                </div>
                {/* Waterfall */}
                <div
                  className="relative bg-zinc-950 rounded overflow-hidden"
                  style={{ height: `${laneCount * 24 + 8}px` }}
                >
                  {/* Vertical grid lines at tick positions */}
                  {ticks.map((tick) => (
                    <div
                      key={tick.pct}
                      className="absolute top-0 bottom-0 border-l border-zinc-800/40"
                      style={{ left: `${tick.pct}%` }}
                    />
                  ))}
                  {/* Span bars */}
                  {session.spans.map((span, i) => {
                    const { lane } = timelineData.lanes[i]
                    const spanStart = new Date(span.started_at).getTime()
                    const left = ((spanStart - timelineData.start) / timelineData.totalMs) * 100
                    const width = Math.max((span.duration_ms / timelineData.totalMs) * 100, 0.5)
                    const laneHeight = 24
                    const top = 4 + lane * laneHeight
                    const barHeight = laneHeight - 4
                    return (
                      <div
                        key={span.id}
                        className={`absolute rounded-sm ${spanColor(span)} transition-opacity hover:opacity-100 opacity-80`}
                        style={{
                          left: `${left}%`,
                          width: `${width}%`,
                          minWidth: '2px',
                          top: `${top}px`,
                          height: `${barHeight}px`,
                        }}
                        title={`#${i + 1} ${span.model} — ${formatDuration(span.duration_ms)}${span.finish_reason ? ` (${span.finish_reason})` : ''}`}
                      />
                    )
                  })}
                </div>
                {/* Legend */}
                <div className="flex flex-wrap gap-3 mt-3 text-[10px] text-zinc-600">
                  <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-sm bg-emerald-500/70" /> {t.detail_legend_success}</span>
                  <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-sm bg-blue-400/60" /> {t.detail_legend_tool}</span>
                  <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-sm bg-amber-500/80" /> {t.detail_legend_4xx}</span>
                  <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-sm bg-red-500/80" /> {t.detail_legend_5xx}</span>
                </div>
              </div>
            )
          })()}

          {/* Spans */}
          <div>
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-xs font-medium text-zinc-500 uppercase tracking-wider">{t.detail_spans_title}</h3>
              <span className="text-xs text-zinc-600 tabular-nums">{tt('detail_spans_total', { count: session.spans.length })}</span>
            </div>
            {session.spans.length === 0 ? (
              <p className="text-sm text-zinc-600 py-8 text-center">{t.detail_no_spans}</p>
            ) : (
              <div className="border-l-2 border-zinc-800/60 ml-2 space-y-0">
                {spansWithGaps.map((span, i) => (
                  <SpanRow key={span.id} span={span} index={i} gapMs={span.gapMs} />
                ))}
              </div>
            )}
          </div>
        </>
      ) : null}
    </div>
  )
}

function SpanRow({ span, index, gapMs }: { span: SpanItem; index: number; gapMs: number }) {
  const { t } = useI18n()
  const [expanded, setExpanded] = useState(false)
  const hasIO = !!span.input || !!span.output

  return (
    <div className="relative pl-6 pb-5 -ml-px animate-fade-in-up" style={{ animationDelay: `${Math.min(index * 40, 400)}ms` }}>
      {/* Inter-span gap indicator */}
      {gapMs > 0 && (
        <div className="absolute -left-[1px] -top-1 flex items-center">
          <div className="w-3 border-t border-dashed border-zinc-700" />
          <span className="text-[9px] text-zinc-600 font-mono tabular-nums ml-1 bg-zinc-950 px-1">{formatGap(gapMs)}</span>
        </div>
      )}

      <div className={`absolute -left-[5px] ${gapMs > 0 ? 'top-5' : 'top-3'} w-2 h-2 rounded-full ${httpStatusDot(span.http_status)} ring-2 ring-zinc-950`} />
      <button
        type="button"
        onClick={() => hasIO && setExpanded(!expanded)}
        className={`w-full text-left ${hasIO ? 'cursor-pointer group' : ''} rounded-lg transition-colors ${gapMs > 0 ? 'mt-3' : ''}`}
      >
        <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4 group-hover:border-zinc-700/80 transition-colors">
          <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-2">
            <div className="flex items-start gap-2">
              {hasIO && (
                <span className="mt-0.5 text-zinc-600 shrink-0 transition-transform duration-150" style={{ transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)' }}>
                  <ChevronRight size={14} />
                </span>
              )}
              <div className="space-y-1">
                <p className="text-[11px] text-zinc-600 font-mono tabular-nums">{formatTimestamp(span.started_at)}</p>
                <div className="flex items-center gap-2">
                  <p className="text-sm text-zinc-200 font-medium">{span.model}</p>
                  {finishReasonBadge(span.finish_reason)}
                </div>
                <p className="text-xs text-zinc-500">
                  {span.input_tokens != null && span.output_tokens != null
                    ? <><span className="tabular-nums">{span.input_tokens}</span> {t.detail_tokens_in} / <span className="tabular-nums">{span.output_tokens}</span> {t.detail_tokens_out}</>
                    : t.detail_tokens_unavailable}
                  <span className="text-zinc-700 mx-1.5">/</span>
                  {formatDuration(span.duration_ms)}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-3 shrink-0">
              <span className="text-sm text-zinc-200 font-mono tabular-nums">{formatCost(span.cost_usd != null ? span.cost_usd : null)}</span>
              <span className={`text-xs font-mono tabular-nums ${httpStatusColor(span.http_status)}`}>{span.http_status}</span>
            </div>
          </div>
        </div>
      </button>

      {expanded && (
        <div className="mt-2 space-y-2 ml-6 animate-fade-in">
          {span.input && (
            <div>
              <p className="text-[10px] font-medium text-zinc-600 uppercase tracking-wider mb-1.5">{t.detail_input}</p>
              <pre className="bg-zinc-950 border border-zinc-800 rounded-md p-3 text-xs text-zinc-400 font-mono overflow-x-auto max-h-80 overflow-y-auto whitespace-pre-wrap break-words leading-relaxed">{span.input}</pre>
            </div>
          )}
          {span.output && (
            <div>
              <p className="text-[10px] font-medium text-zinc-600 uppercase tracking-wider mb-1.5">{t.detail_output}</p>
              <pre className="bg-zinc-950 border border-zinc-800 rounded-md p-3 text-xs text-zinc-400 font-mono overflow-x-auto max-h-80 overflow-y-auto whitespace-pre-wrap break-words leading-relaxed">{span.output}</pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
