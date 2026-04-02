import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from 'recharts'
import { formatDay } from '@/lib/date'
import type { DailyStatsRow } from '@/types/api'

interface DailyChartProps {
  data: DailyStatsRow[]
}

interface TooltipPayload {
  value: number
  name: string
}

interface CustomTooltipProps {
  active?: boolean
  payload?: TooltipPayload[]
  label?: string
}

function CustomTooltip({ active, payload, label }: CustomTooltipProps) {
  if (!active || !payload || payload.length === 0) return null
  return (
    <div className="bg-zinc-900 border border-zinc-700/50 rounded-md px-3 py-2 text-xs shadow-lg shadow-black/20">
      <p className="text-zinc-500 mb-1">{label}</p>
      <p>
        <span className="text-zinc-100 font-medium tabular-nums">{payload[0].value}</span>{' '}
        <span className="text-zinc-500">sessions</span>
      </p>
    </div>
  )
}

export function DailyChart({ data }: DailyChartProps) {
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
            <Bar
              dataKey="session_count"
              fill="var(--chart-1)"
              radius={[3, 3, 0, 0]}
              isAnimationActive
              animationDuration={600}
              animationEasing="ease-out"
            />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
