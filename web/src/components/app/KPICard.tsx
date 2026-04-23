import { cn } from '@/lib/utils'

interface KPICardProps {
  title: string
  value: string
  icon?: React.ComponentType<{ size?: number; className?: string }>
  alert?: boolean
  className?: string
}

export function KPICard({ title, value, icon: Icon, alert, className }: KPICardProps) {
  return (
    <div className={cn(
      'relative bg-zinc-900 border border-zinc-800 rounded-lg p-4 overflow-hidden',
      'transition-colors duration-150 hover:border-zinc-700/80',
      className
    )}>
      <div className="flex items-center justify-between mb-3">
        <span className="text-xs text-zinc-500 uppercase tracking-wider font-medium">{title}</span>
        {Icon && <Icon size={14} className="text-zinc-700" />}
      </div>
      <div className={cn(
        'text-2xl font-semibold tracking-tight tabular-nums',
        alert ? 'text-red-400' : 'text-zinc-50'
      )}>
        {value}
      </div>
      {alert && (
        <div className="absolute bottom-0 left-0 right-0 h-px bg-gradient-to-r from-transparent via-red-500/40 to-transparent" />
      )}
    </div>
  )
}
