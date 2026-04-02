import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { StatsResult, DailyStatsRow, AgentStatsRow, FinishReasonCount } from '@/types/api'

function toDateKey(d: Date): string {
  return d.toISOString().slice(0, 10)
}

export function useStats(orgID: string, from: Date, to: Date) {
  return useQuery({
    queryKey: ['stats', orgID, toDateKey(from), toDateKey(to)],
    queryFn: () =>
      api.get<StatsResult>(
        `/api/orgs/${orgID}/stats?from=${from.toISOString()}&to=${to.toISOString()}`
      ),
    enabled: !!orgID,
  })
}

export function useDailyStats(orgID: string, from: Date, to: Date) {
  return useQuery({
    queryKey: ['dailyStats', orgID, toDateKey(from), toDateKey(to)],
    queryFn: () =>
      api.get<DailyStatsRow[]>(
        `/api/orgs/${orgID}/stats/daily?from=${from.toISOString()}&to=${to.toISOString()}`
      ),
    enabled: !!orgID,
  })
}

export function useAgentStats(orgID: string, from: Date, to: Date) {
  return useQuery({
    queryKey: ['agentStats', orgID, toDateKey(from), toDateKey(to)],
    queryFn: () =>
      api.get<AgentStatsRow[]>(
        `/api/orgs/${orgID}/stats/agents?from=${from.toISOString()}&to=${to.toISOString()}`
      ),
    enabled: !!orgID,
  })
}

export function useFinishReasons(orgID: string, from: Date, to: Date) {
  return useQuery({
    queryKey: ['finishReasons', orgID, toDateKey(from), toDateKey(to)],
    queryFn: () =>
      api.get<FinishReasonCount[]>(
        `/api/orgs/${orgID}/stats/finish-reasons?from=${from.toISOString()}&to=${to.toISOString()}`
      ),
    enabled: !!orgID,
  })
}
