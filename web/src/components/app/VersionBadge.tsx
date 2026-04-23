import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'

interface MetaResponse {
  version: string
}

export function VersionBadge() {
  const { data } = useQuery({
    queryKey: ['meta'],
    queryFn: () => api.get<MetaResponse>('/meta'),
    staleTime: Infinity,
    retry: false,
  })

  if (!data?.version) return null

  return (
    <div
      aria-hidden
      className="pointer-events-none fixed bottom-1.5 right-2 z-50 select-none text-[10px] font-mono tracking-tight text-zinc-600/70"
    >
      {data.version}
    </div>
  )
}
