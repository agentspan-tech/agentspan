import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { formatDay } from '@/lib/date'
import { useI18n } from '@/i18n'
import type { DailyStatsRow } from '@/types/api'

interface DailyChartProps {
  data: DailyStatsRow[]
}

// Bar fills — rich saturated tones on zinc-900
const STATUS_COLORS = {
  completed: '#1a8a5e',       // green
  with_errors: '#b8722a',     // orange
  failed: '#b04040',          // red
  abandoned: '#6b6b7a',       // gray
  in_progress: '#3d74b8',     // blue
} as const

// Tooltip dots — same hues, lighter for legibility on dark bg
const STATUS_DOT_COLORS = {
  completed: '#34d399',       // green
  with_errors: '#e0994a',     // orange
  failed: '#d45858',          // red
  abandoned: '#9090a0',       // gray
  in_progress: '#5a9ae0',     // blue
} as const

type StatusKey = keyof typeof STATUS_COLORS

interface TooltipEntry {
  value: number
  name: string
  color: string
  dataKey: string
}

interface CustomTooltipProps {
  active?: boolean
  payload?: TooltipEntry[]
  label?: string
}

function CustomTooltip({ active, payload, label }: CustomTooltipProps) {
  if (!active || !payload || payload.length === 0) return null
  const visible = payload.filter(e => e.value > 0)
  if (visible.length === 0) return null
  const total = visible.reduce((sum, entry) => sum + entry.value, 0)
  return (
    <div className="bg-zinc-900 border border-zinc-700/50 rounded-md px-3 py-2.5 text-xs shadow-lg shadow-black/20">
      <p className="text-zinc-500 mb-2">{label}</p>
      {visible.map((entry) => {
        const dotKey = Object.keys(STATUS_COLORS).find(
          k => STATUS_COLORS[k as StatusKey] === entry.color
        ) as StatusKey | undefined
        return (
          <div key={entry.dataKey} className="flex items-center gap-2 py-0.5">
            <span className="w-2 h-2 rounded-full shrink-0" style={{ backgroundColor: dotKey ? STATUS_DOT_COLORS[dotKey] : entry.color }} />
            <span className="text-zinc-400">{entry.name}</span>
            <span className="text-zinc-100 font-medium tabular-nums ml-auto pl-3">{entry.value}</span>
          </div>
        )
      })}
      {visible.length > 1 && (
        <div className="flex items-center gap-2 pt-1.5 mt-1.5 border-t border-zinc-800">
          <span className="text-zinc-500">Total</span>
          <span className="text-zinc-100 font-medium tabular-nums ml-auto pl-3">{total}</span>
        </div>
      )}
    </div>
  )
}

export function DailyChart({ data }: DailyChartProps) {
  const { t } = useI18n()

  const statusKeys: { key: StatusKey; dataKey: string; label: string }[] = [
    { key: 'completed', dataKey: 'completed_count', label: t.sessions_badge_completed },
    { key: 'with_errors', dataKey: 'with_errors_count', label: t.sessions_badge_with_errors },
    { key: 'failed', dataKey: 'failed_count', label: t.sessions_badge_failed },
    { key: 'abandoned', dataKey: 'abandoned_count', label: t.sessions_badge_abandoned },
    { key: 'in_progress', dataKey: 'in_progress_count', label: t.sessions_badge_in_progress },
  ]

  return (
    <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-5">
      <div className="flex items-center justify-between mb-5">
        <h3 className="text-xs font-medium text-zinc-500 uppercase tracking-wider">Daily Activity</h3>
        <span className="text-xs text-zinc-600 tabular-nums">{data.length} days</span>
      </div>
      <div className="h-[260px]">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} barCategoryGap="40%">
            <XAxis
              dataKey="day"
              tickFormatter={formatDay}
              tick={{ fill: '#52525b', fontSize: 11 }}
              axisLine={false}
              tickLine={false}
            />
            <YAxis
              allowDecimals={false}
              tick={{ fill: '#52525b', fontSize: 11 }}
              axisLine={false}
              tickLine={false}
              width={32}
            />
            <Tooltip
              content={<CustomTooltip />}
              cursor={{ fill: 'rgba(99, 102, 241, 0.04)' }}
            />
            {statusKeys.map(({ key, dataKey, label }, i) => (
              <Bar
                key={key}
                dataKey={dataKey}
                name={label}
                stackId="sessions"
                fill={STATUS_COLORS[key]}
                radius={i === statusKeys.length - 1 ? [3, 3, 0, 0] : [0, 0, 0, 0]}
                isAnimationActive
                animationDuration={600}
                animationEasing="ease-out"
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
