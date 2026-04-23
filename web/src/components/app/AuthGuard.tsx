import { Navigate, Outlet } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { useAuthStore } from '@/store'
import { api } from '@/lib/api'

export function AuthGuard() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const setAuthenticated = useAuthStore((s) => s.setAuthenticated)

  // On mount, verify session with the server if not already authenticated.
  // This restores auth state after page refresh (the httpOnly cookie is still present).
  const { isLoading, isError } = useQuery({
    queryKey: ['session-check'],
    queryFn: async () => {
      const result = await api.get<unknown[]>('/api/orgs/')
      setAuthenticated(true)
      return result
    },
    enabled: !isAuthenticated,
    retry: false,
    staleTime: Infinity,
  })

  if (isAuthenticated) return <Outlet />

  // Still checking session — show nothing (prevents flash of login page).
  if (isLoading) return null

  // Session check failed — redirect to login.
  if (isError) return <Navigate to="/login" replace />

  return <Navigate to="/login" replace />
}
