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

  // Lives in the normal flow at the end of its parent. Pair with a `flex flex-col`
  // `min-h-screen` parent so `mt-auto` pins the badge to the bottom of the viewport
  // when content is short, and to the bottom of the content when it's longer —
  // never overlapping anything.
  return (
    <div
      aria-hidden
      className="mt-auto flex justify-end pointer-events-none select-none px-2 pb-1.5 text-[10px] font-mono tracking-tight text-zinc-600/70"
    >
      {data.version}
    </div>
  )
}
