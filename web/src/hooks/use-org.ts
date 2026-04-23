import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import type { Organization, OrgMember, Invite, AlertRule, PrivacySettings, MaskingConfig } from '@/types/api'

// Org detail
export function useOrg(orgID: string) {
  return useQuery({
    queryKey: ['org', orgID],
    queryFn: () => api.get<Organization>(`/api/orgs/${orgID}`),
    enabled: !!orgID,
  })
}

// Update settings
export function useUpdateOrgSettings(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { name?: string; locale?: string; session_timeout_seconds?: number }) =>
      api.put<Organization>(`/api/orgs/${orgID}/settings`, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['org', orgID] }),
  })
}

// Members
export function useOrgMembers(orgID: string) {
  return useQuery({
    queryKey: ['members', orgID],
    queryFn: () => api.get<OrgMember[]>(`/api/orgs/${orgID}/members`),
    enabled: !!orgID,
  })
}

export function useRemoveMember(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (memberID: string) =>
      api.delete(`/api/orgs/${orgID}/members/${memberID}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['members', orgID] }),
  })
}

// Invites
export function useInvites(orgID: string) {
  return useQuery({
    queryKey: ['invites', orgID],
    queryFn: () => api.get<Invite[]>(`/api/orgs/${orgID}/invites`),
    enabled: !!orgID,
  })
}

export function useCreateInvite(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { email: string; role: string }) =>
      api.post<Invite>(`/api/orgs/${orgID}/invites`, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['invites', orgID] }),
  })
}

export function useRevokeInvite(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (inviteID: string) =>
      api.delete(`/api/orgs/${orgID}/invites/${inviteID}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['invites', orgID] }),
  })
}

// Alerts
export function useAlertRules(orgID: string) {
  return useQuery({
    queryKey: ['alerts', orgID],
    queryFn: () => api.get<AlertRule[]>(`/api/orgs/${orgID}/alerts`),
    enabled: !!orgID,
    retry: (count, error) => {
      // Don't retry 403 (tier-gated)
      if (error instanceof ApiError && error.status === 403) return false
      return count < 2
    },
  })
}

export function useCreateAlertRule(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: {
      name: string
      alert_type: string
      threshold: number
      window_minutes: number
      cooldown_minutes: number
      notify_roles: string[]
      enabled: boolean
    }) => api.post<AlertRule>(`/api/orgs/${orgID}/alerts`, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['alerts', orgID] }),
  })
}

export function useUpdateAlertRule(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ alertID, data }: {
      alertID: string
      data: Partial<{
        name: string
        alert_type: string
        threshold: number
        window_minutes: number
        cooldown_minutes: number
        notify_roles: string[]
        enabled: boolean
      }>
    }) => api.put<AlertRule>(`/api/orgs/${orgID}/alerts/${alertID}`, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['alerts', orgID] }),
  })
}

export function useDeleteAlertRule(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (alertID: string) =>
      api.delete(`/api/orgs/${orgID}/alerts/${alertID}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['alerts', orgID] }),
  })
}

// Privacy settings
export function usePrivacySettings(orgID: string) {
  return useQuery({
    queryKey: ['privacy-settings', orgID],
    queryFn: () => api.get<PrivacySettings>(`/api/orgs/${orgID}/privacy-settings`),
    enabled: !!orgID,
  })
}

export function useUpdatePrivacySettings(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { store_span_content: boolean; masking_config: MaskingConfig }) =>
      api.put(`/api/orgs/${orgID}/privacy-settings`, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['privacy-settings', orgID] }),
  })
}

// Org deletion
export function useInitiateDeletion(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.delete(`/api/orgs/${orgID}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['org', orgID] }),
  })
}

export function useCancelDeletion(orgID: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api.post(`/api/orgs/${orgID}/restore`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['org', orgID] }),
  })
}
