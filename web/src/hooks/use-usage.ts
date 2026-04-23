import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { UsageResponse } from '@/types/api'

export function useUsage(orgID: string) {
  return useQuery({
    queryKey: ['usage', orgID],
    queryFn: () => api.get<UsageResponse>(`/api/orgs/${orgID}/usage`),
    enabled: !!orgID,
    staleTime: 30_000,
  })
}
