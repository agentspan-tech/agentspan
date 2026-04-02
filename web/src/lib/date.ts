import { subDays, subHours, startOfDay, format, parseISO, formatDistanceToNow } from 'date-fns'

export type DateRange = '24h' | '7d' | '30d' | 'custom'

export function getDateRange(range: DateRange): { from: Date; to: Date } {
  // Zero milliseconds so queryKey stays stable within the same second across re-renders
  const now = new Date()
  now.setMilliseconds(0)
  switch (range) {
    case '24h': return { from: subHours(now, 24), to: now }
    case '7d': return { from: startOfDay(subDays(now, 6)), to: now }
    case '30d': return { from: startOfDay(subDays(now, 29)), to: now }
    default: return { from: startOfDay(subDays(now, 6)), to: now }
  }
}

export function formatDay(isoDate: string): string {
  return format(parseISO(isoDate), 'MMM d')
}

export function formatTimestamp(iso: string): string {
  return format(parseISO(iso), 'MMM d, HH:mm:ss')
}

export function formatRelative(iso: string): string {
  return formatDistanceToNow(parseISO(iso), { addSuffix: true })
}

export function formatCost(usd: number | null): string {
  if (usd === null) return '--'
  return `$${usd.toFixed(4)}`
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

export function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return n.toString()
}
