import { useI18n } from '@/i18n'
import type { SessionStatus } from '@/types/api'

interface SessionStatusBadgeProps {
  status: SessionStatus
}

export function SessionStatusBadge({ status }: SessionStatusBadgeProps) {
  const { t } = useI18n()

  const STATUS_CONFIG: Record<SessionStatus, { label: string; text: string; bg: string; dot: string }> = {
    in_progress: { label: t.sessions_badge_in_progress, text: 'text-blue-400', bg: 'bg-blue-500/10', dot: 'bg-blue-400' },
    completed: { label: t.sessions_badge_completed, text: 'text-emerald-400', bg: 'bg-emerald-500/10', dot: 'bg-emerald-400' },
    completed_with_errors: { label: t.sessions_badge_with_errors, text: 'text-orange-400', bg: 'bg-orange-500/10', dot: 'bg-orange-400' },
    failed: { label: t.sessions_badge_failed, text: 'text-red-400', bg: 'bg-red-500/10', dot: 'bg-red-400' },
    abandoned: { label: t.sessions_badge_abandoned, text: 'text-amber-400', bg: 'bg-amber-500/10', dot: 'bg-amber-400' },
  }

  const config = STATUS_CONFIG[status]
  return (
    <span className={`inline-flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wider px-2 py-0.5 rounded ${config.text} ${config.bg}`}>
      <span className={`w-1 h-1 rounded-full ${config.dot} ${status === 'in_progress' ? 'animate-pulse' : ''}`} />
      {config.label}
    </span>
  )
}
