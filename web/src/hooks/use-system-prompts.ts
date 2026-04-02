import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { SystemPromptListItem, SystemPromptDetail } from '@/types/api'

export function useSystemPrompts(orgID: string) {
  return useQuery({
    queryKey: ['systemPrompts', orgID],
    queryFn: () =>
      api.get<SystemPromptListItem[]>(`/api/orgs/${orgID}/system-prompts`),
    enabled: !!orgID,
  })
}

export function useSystemPrompt(orgID: string, promptID: string | null) {
  return useQuery({
    queryKey: ['systemPrompt', orgID, promptID],
    queryFn: () =>
      api.get<SystemPromptDetail>(`/api/orgs/${orgID}/system-prompts/${promptID}`),
    enabled: !!orgID && !!promptID,
  })
}
