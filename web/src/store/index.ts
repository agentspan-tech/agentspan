import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthState {
  isAuthenticated: boolean
  setAuthenticated: (auth: boolean) => void
  activeOrgID: string | null
  setActiveOrgID: (id: string | null) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      isAuthenticated: false,
      setAuthenticated: (auth) => set({ isAuthenticated: auth }),
      activeOrgID: null,
      setActiveOrgID: (id) => set({ activeOrgID: id }),
      logout: () => set({ isAuthenticated: false, activeOrgID: null }),
    }),
    {
      name: 'agentorbit-auth',
      // Only persist activeOrgID (user preference). isAuthenticated is derived
      // from server state on page load — never trust it from localStorage.
      partialize: (state) => ({ activeOrgID: state.activeOrgID }),
    }
  )
)
