import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { APIKeyListItem, APIKeyCreateResult, APIKeyCreateRequest } from '@/types/api'

export function useAPIKeys(orgID: string) {
  return useQuery({
    queryKey: ['keys', orgID],
    queryFn: () => api.get<APIKeyListItem[]>(`/api/orgs/${orgID}/keys`),
    enabled: !!orgID,
  })
}

export function useCreateAPIKey(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: APIKeyCreateRequest) =>
      api.post<APIKeyCreateResult>(`/api/orgs/${orgID}/keys`, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['keys', orgID] }),
  })
}

export function useDeactivateAPIKey(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (keyID: string) =>
      api.post<void>(`/api/orgs/${orgID}/keys/${keyID}/deactivate`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['keys', orgID] }),
  })
}
