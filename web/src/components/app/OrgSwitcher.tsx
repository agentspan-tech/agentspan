import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { useI18n } from '@/i18n'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ChevronDown, Plus } from 'lucide-react'
import { api } from '@/lib/api'
import { useAuthStore } from '@/store'
import type { Organization } from '@/types/api'

export function OrgSwitcher() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { activeOrgID, setActiveOrgID } = useAuthStore()

  const handleOrgSwitch = (orgId: string) => {
    setActiveOrgID(orgId)
    // Invalidate org-scoped queries; orgs list itself stays cached
    queryClient.invalidateQueries({ queryKey: ['sessions'] })
    queryClient.invalidateQueries({ queryKey: ['stats'] })
    queryClient.invalidateQueries({ queryKey: ['dailyStats'] })
    queryClient.invalidateQueries({ queryKey: ['keys'] })
    queryClient.invalidateQueries({ queryKey: ['alerts'] })
    queryClient.invalidateQueries({ queryKey: ['members'] })
    queryClient.invalidateQueries({ queryKey: ['invites'] })
  }

  const { data: orgs } = useQuery<Organization[]>({
    queryKey: ['orgs'],
    queryFn: () => api.get<Organization[]>('/api/orgs/'),
    enabled: useAuthStore.getState().isAuthenticated,
  })

  const activeOrg = orgs?.find((o) => o.id === activeOrgID) ?? orgs?.[0]

  const initials = (name: string) =>
    name.split(/\s+/).map((w) => w[0]).join('').toUpperCase().slice(0, 2)

  if (!activeOrg) {
    return (
      <div className="flex items-center gap-2 px-1 py-1 text-sm text-zinc-500">
        <div className="h-6 w-6 rounded bg-zinc-800 animate-pulse" />
        <span className="truncate">{t.org_switcher_loading}</span>
      </div>
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button className="w-full flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-zinc-900 transition-colors text-sm text-zinc-300">
          <div className="h-6 w-6 rounded bg-zinc-800 flex items-center justify-center text-[10px] font-medium text-zinc-400 shrink-0">
            {initials(activeOrg.name)}
          </div>
          <span className="truncate flex-1 text-left">{activeOrg.name}</span>
          <ChevronDown size={12} className="text-zinc-600 shrink-0" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-52 bg-zinc-900 border-zinc-800">
        {(orgs ?? []).map((org) => (
          <DropdownMenuItem
            key={org.id}
            onClick={() => handleOrgSwitch(org.id)}
            className={org.id === activeOrg.id ? 'text-zinc-50 bg-zinc-800' : 'text-zinc-400 hover:bg-zinc-800'}
          >
            <div className="h-5 w-5 rounded bg-zinc-800 flex items-center justify-center text-[9px] font-medium text-zinc-400 mr-2 shrink-0">
              {initials(org.name)}
            </div>
            <span className="truncate">{org.name}</span>
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator className="bg-zinc-800" />
        <DropdownMenuItem
          onClick={() => navigate('/create-org')}
          className="text-zinc-400 hover:bg-zinc-800"
        >
          <Plus size={14} className="mr-2 shrink-0" />
          <span>{t.org_switcher_create}</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
