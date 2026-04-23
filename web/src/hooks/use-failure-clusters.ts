import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { FailureClusterItem, ClusterSessionItem } from '@/types/api'

export function useFailureClusters(orgID: string) {
  return useQuery({
    queryKey: ['failureClusters', orgID],
    queryFn: () =>
      api.get<FailureClusterItem[]>(`/api/orgs/${orgID}/failure-clusters`),
    enabled: !!orgID,
  })
}

export function useClusterSessions(orgID: string, clusterID: string | null) {
  return useQuery({
    queryKey: ['clusterSessions', orgID, clusterID],
    queryFn: () =>
      api.get<ClusterSessionItem[]>(`/api/orgs/${orgID}/failure-clusters/${clusterID}/sessions`),
    enabled: !!orgID && !!clusterID,
  })
}
