import { useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { useI18n } from '@/i18n'
import { useUsage } from '@/hooks/use-usage'
import { useOrg } from '@/hooks/use-org'
import { useAuthStore } from '@/store'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'

export function UsageBar({ collapsed }: { collapsed: boolean }) {
  const { t } = useI18n()
  const orgID = useAuthStore((s) => s.activeOrgID) ?? ''
  const { data: org } = useOrg(orgID)
  const { data: usage } = useUsage(orgID)
  const warningShownRef = useRef(false)

  const isFree = org?.plan === 'free'
  const used = usage?.spans_used ?? 0
  const limit = usage?.spans_limit ?? 0
  const pct = limit > 0 ? Math.min((used / limit) * 100, 100) : 0

  const color = pct >= 95 ? 'red' : pct >= 80 ? 'amber' : 'blue'
  const barColor = color === 'red' ? 'bg-red-500' : color === 'amber' ? 'bg-amber-500' : 'bg-blue-500'
  const textColor = color === 'red' ? 'text-red-500' : color === 'amber' ? 'text-amber-500' : 'text-zinc-500'
  const countColor = color === 'red' ? 'text-red-500 font-semibold' : color === 'amber' ? 'text-amber-500 font-semibold' : 'text-zinc-300'
  const linkColor = color === 'red' ? 'text-red-500 font-semibold' : color === 'amber' ? 'text-amber-500 font-medium' : 'text-blue-500'

  // Toast at 80% threshold, once per browser session.
  useEffect(() => {
    if (!isFree || !usage || warningShownRef.current) return
    if (pct >= 80 && !sessionStorage.getItem('usageWarningShown')) {
      sessionStorage.setItem('usageWarningShown', '1')
      warningShownRef.current = true
      toast.warning(t.sidebar_usage_warning)
    }
  }, [isFree, usage, pct, t])

  if (!isFree || !usage) return null

  if (collapsed) {
    return (
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <div className="px-3 pb-2">
              <div className="h-1 bg-zinc-800 rounded-full overflow-hidden">
                <div className={cn('h-full rounded-full transition-all', barColor)} style={{ width: `${pct}%` }} />
              </div>
            </div>
          </TooltipTrigger>
          <TooltipContent side="right">
            <span>{used.toLocaleString()} / {limit.toLocaleString()} spans</span>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    )
  }

  return (
    <div className="px-4 pb-3 space-y-1.5">
      <div className="flex items-center justify-between text-[11px]">
        <span className={textColor}>{t.sidebar_usage_free_plan}</span>
        <span className={countColor}>{used.toLocaleString()} / {limit.toLocaleString()}</span>
      </div>
      <div className="h-1.5 bg-zinc-800 rounded-full overflow-hidden">
        <div className={cn('h-full rounded-full transition-all', barColor)} style={{ width: `${pct}%` }} />
      </div>
      <Link to="/settings" className={cn('block text-[11px] transition-colors hover:opacity-80', linkColor)}>
        {t.sidebar_usage_upgrade} →
      </Link>
    </div>
  )
}
