import { describe, it, expect, beforeEach } from 'vitest'
import { useAuthStore } from '@/store'

describe('useAuthStore', () => {
  beforeEach(() => {
    useAuthStore.setState({ isAuthenticated: false, activeOrgID: null })
  })

  it('stores and retrieves authenticated state', () => {
    useAuthStore.getState().setAuthenticated(true)
    expect(useAuthStore.getState().isAuthenticated).toBe(true)
  })

  it('stores active org ID', () => {
    useAuthStore.getState().setActiveOrgID('org-123')
    expect(useAuthStore.getState().activeOrgID).toBe('org-123')
  })

  it('logout clears auth state and org', () => {
    useAuthStore.getState().setAuthenticated(true)
    useAuthStore.getState().setActiveOrgID('org-1')
    useAuthStore.getState().logout()
    expect(useAuthStore.getState().isAuthenticated).toBe(false)
    expect(useAuthStore.getState().activeOrgID).toBeNull()
  })
})
