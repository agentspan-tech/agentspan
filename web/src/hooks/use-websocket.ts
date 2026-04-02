import { useEffect } from 'react'
import { create } from 'zustand'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/store'
import { toast } from 'sonner'

// ---- WebSocket connection state store ----

type WSStatus = 'connecting' | 'connected' | 'disconnected'

interface WSState {
  status: WSStatus
  setStatus: (status: WSStatus) => void
}

export const useWSStore = create<WSState>((set) => ({
  status: 'disconnected',
  setStatus: (status) => set({ status }),
}))

// ---- useSessionsSocket ----
// Subscribes to SessionsChannel for org-level events (session.created, session.updated, alert.triggered)
// Wires into TanStack Query via invalidateQueries on events.

export function useSessionsSocket(orgID: string) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!orgID || !isAuthenticated) return

    const MAX_RECONNECT_ATTEMPTS = 20
    let ws: WebSocket | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let connectDebounceTimer: ReturnType<typeof setTimeout> | null = null
    let attempt = 0
    let destroyed = false
    const subscriptions: object[] = []

    function scheduleReconnect() {
      if (destroyed || attempt >= MAX_RECONNECT_ATTEMPTS) return
      const delay = Math.min(500 * Math.pow(2, attempt), 30000)
      attempt++
      reconnectTimer = setTimeout(connect, delay)
    }

    function connect() {
      if (destroyed) return
      useWSStore.getState().setStatus('connecting')

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      ws = new WebSocket(`${protocol}//${window.location.host}/cable`)

      // Connection timeout: close if still CONNECTING after 10s
      const connectTimeout = setTimeout(() => {
        if (ws && ws.readyState === WebSocket.CONNECTING) {
          ws.close()
        }
      }, 10000)

      ws.onopen = () => {
        clearTimeout(connectTimeout)
        attempt = 0
        useWSStore.getState().setStatus('connected')
        // Auth is handled via httpOnly cookie on the upgrade request.
        // Subscribe to SessionsChannel
        const sub = { command: 'subscribe', identifier: { channel: 'SessionsChannel', org_id: orgID } }
        ws?.send(JSON.stringify(sub))
        subscriptions.push(sub.identifier)
      }

      ws.onmessage = (e) => {
        let msg: any
        try { msg = JSON.parse(e.data) } catch { return }
        if (msg.type === 'confirm_subscription') return

        // Invalidate relevant queries based on event type
        if (msg.type === 'session.created' || msg.type === 'session.updated') {
          queryClient.invalidateQueries({ queryKey: ['sessions', orgID] })
          // Refetch stats immediately so KPI cards and daily chart reflect new data.
          queryClient.invalidateQueries({ queryKey: ['stats', orgID] })
          queryClient.invalidateQueries({ queryKey: ['dailyStats', orgID] })
        }
        if (msg.type === 'alert.triggered') {
          // Toast for alert events per UI-SPEC: "Alert triggered: [rule name]"
          toast(`Alert triggered: ${msg.payload?.rule_name ?? 'unknown rule'}`)
        }
      }

      ws.onclose = () => {
        clearTimeout(connectTimeout)
        useWSStore.getState().setStatus('disconnected')
        if (!destroyed) scheduleReconnect()
      }

      ws.onerror = () => {
        ws?.close()
        // Ensure reconnect fires even if onclose doesn't trigger after onerror
        if (!destroyed && !reconnectTimer) {
          useWSStore.getState().setStatus('disconnected')
          scheduleReconnect()
        }
      }
    }

    // Debounce initial connection by 100ms to handle rapid prop changes
    connectDebounceTimer = setTimeout(connect, 100)

    return () => {
      destroyed = true
      if (connectDebounceTimer) clearTimeout(connectDebounceTimer)
      if (reconnectTimer) clearTimeout(reconnectTimer)
      if (ws && ws.readyState === WebSocket.OPEN) {
        subscriptions.forEach((id) => {
          ws?.send(JSON.stringify({ command: 'unsubscribe', identifier: id }))
        })
        ws.close()
      }
      useWSStore.getState().setStatus('disconnected')
    }
  }, [orgID, isAuthenticated, queryClient])
}

// ---- useSessionSocket ----
// Subscribes to SessionChannel for session-level span.created events.
// Per WS-03/SEC-03: span payloads contain only metrics (no I/O content).

export function useSessionSocket(sessionID: string) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const activeOrgID = useAuthStore((s) => s.activeOrgID)
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!sessionID || !isAuthenticated || !activeOrgID) return

    // Capture orgID at effect creation time to use consistently in this effect.
    const orgID = activeOrgID
    const MAX_RECONNECT_ATTEMPTS = 20
    let ws: WebSocket | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let connectDebounceTimer: ReturnType<typeof setTimeout> | null = null
    let attempt = 0
    let destroyed = false

    function scheduleReconnect() {
      if (destroyed || attempt >= MAX_RECONNECT_ATTEMPTS) return
      const delay = Math.min(500 * Math.pow(2, attempt), 30000)
      attempt++
      reconnectTimer = setTimeout(connect, delay)
    }

    function connect() {
      if (destroyed) return

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      ws = new WebSocket(`${protocol}//${window.location.host}/cable`)

      // Connection timeout: close if still CONNECTING after 10s
      const connectTimeout = setTimeout(() => {
        if (ws && ws.readyState === WebSocket.CONNECTING) {
          ws.close()
        }
      }, 10000)

      ws.onopen = () => {
        clearTimeout(connectTimeout)
        attempt = 0
        // Auth is handled via httpOnly cookie on the upgrade request.
        // Subscribe to SessionChannel for this specific session
        const sub = { command: 'subscribe', identifier: { channel: 'SessionChannel', session_id: sessionID } }
        ws?.send(JSON.stringify(sub))
      }

      ws.onmessage = (e) => {
        let msg: any
        try { msg = JSON.parse(e.data) } catch { return }
        if (msg.type === 'confirm_subscription') return

        if (msg.type === 'span.created') {
          // Invalidate session detail query to refetch with new span.
          // Uses captured orgID to avoid stale closure issues.
          queryClient.invalidateQueries({ queryKey: ['session', orgID, sessionID] })
        }
      }

      ws.onclose = () => {
        clearTimeout(connectTimeout)
        if (!destroyed) scheduleReconnect()
      }

      ws.onerror = () => {
        ws?.close()
        // Ensure reconnect fires even if onclose doesn't trigger after onerror
        if (!destroyed && !reconnectTimer) {
          scheduleReconnect()
        }
      }
    }

    // Debounce initial connection by 100ms to handle rapid prop changes
    connectDebounceTimer = setTimeout(connect, 100)

    return () => {
      destroyed = true
      if (connectDebounceTimer) clearTimeout(connectDebounceTimer)
      if (reconnectTimer) clearTimeout(reconnectTimer)
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ command: 'unsubscribe', identifier: { channel: 'SessionChannel', session_id: sessionID } }))
        ws.close()
      }
    }
  }, [sessionID, isAuthenticated, activeOrgID, queryClient])
}
