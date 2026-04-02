import { describe, it, expect } from 'vitest'
import { useWSStore } from './use-websocket'

describe('useWSStore', () => {
  it('initializes as disconnected', () => {
    expect(useWSStore.getState().status).toBe('disconnected')
  })

  it('updates status', () => {
    useWSStore.getState().setStatus('connected')
    expect(useWSStore.getState().status).toBe('connected')
    useWSStore.getState().setStatus('disconnected')
  })
})
