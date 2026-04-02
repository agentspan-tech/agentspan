import { useQuery, useInfiniteQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { SessionListItem, SessionDetail, PaginatedResponse } from '@/types/api'

export interface SessionFilters {
  status?: string
  agent_name?: string
  api_key_id?: string
  provider_type?: string
}

export function useSessions(orgID: string, filters: SessionFilters) {
  return useInfiniteQuery({
    queryKey: ['sessions', orgID, filters],
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams()
      if (filters.status) params.set('status', filters.status)
      if (filters.agent_name) params.set('agent_name', filters.agent_name)
      if (filters.api_key_id) params.set('api_key_id', filters.api_key_id)
      if (filters.provider_type) params.set('provider_type', filters.provider_type)
      if (pageParam) params.set('cursor', pageParam)
      params.set('limit', '20')
      return api.get<PaginatedResponse<SessionListItem>>(
        `/api/orgs/${orgID}/sessions?${params.toString()}`
      )
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    initialPageParam: undefined as string | undefined,
    enabled: !!orgID,
  })
}

export function useSession(orgID: string, sessionID: string) {
  return useQuery({
    queryKey: ['session', orgID, sessionID],
    queryFn: () =>
      api.get<SessionDetail>(`/api/orgs/${orgID}/sessions/${sessionID}`),
    enabled: !!orgID && !!sessionID,
  })
}
