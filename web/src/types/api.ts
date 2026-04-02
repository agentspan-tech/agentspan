// Session types
export type SessionStatus = 'in_progress' | 'completed' | 'completed_with_errors' | 'failed' | 'abandoned'

export interface SessionListItem {
  id: string
  api_key_id: string
  api_key_name: string
  external_id?: string
  agent_name?: string
  status: SessionStatus
  total_cost_usd: number | null
  span_count: number
  started_at: string
  last_span_at: string
  closed_at?: string
}

export interface SessionDetail extends Omit<SessionListItem, 'span_count'> {
  narrative: string | null
  span_count: number
  spans: SpanItem[]
}

export interface SpanItem {
  id: string
  provider_type: string
  model: string
  input?: string
  output?: string
  input_tokens?: number
  output_tokens?: number
  cost_usd?: number
  duration_ms: number
  http_status: number
  finish_reason?: string
  started_at: string
  created_at: string
}

// Stats types
export interface StatsResult {
  total_sessions: number
  total_spans: number
  total_cost_usd: number
  avg_duration_ms: number
  error_rate: number
}

export interface AgentStatsRow {
  api_key_id: string
  api_key_name: string
  session_count: number
  span_count: number
  total_cost_usd: number
  avg_duration_ms: number
  error_rate: number
  avg_token_ratio: number
}

export interface FinishReasonCount {
  finish_reason: string
  count: number
}

export interface DailyStatsRow {
  day: string
  session_count: number
  span_count: number
  cost_usd: number
}

// API Key types
export interface APIKeyListItem {
  id: string
  name: string
  provider_type: string
  display: string
  active: boolean
  last_used_at: string | null
  created_at: string
}

export interface APIKeyCreateResult extends APIKeyListItem {
  raw_key: string
}

export interface APIKeyCreateRequest {
  name: string
  provider_type: string
  provider_key: string
  base_url?: string
}

// Organization types
// sql.NullTime serializes as { Time: string, Valid: boolean }
export interface NullTime {
  Time: string
  Valid: boolean
}

export interface Organization {
  id: string
  name: string
  plan: 'free' | 'pro' | 'selfhost'
  locale: string
  session_timeout_seconds: number
  created_at: string
  deletion_scheduled_at: NullTime
}

export interface OrgMember {
  user_id: string
  user_name: string
  email: string
  role: 'owner' | 'admin' | 'member' | 'viewer'
  created_at: string
}

export interface Invite {
  id: string
  email: string
  role: string
  status: string
  created_at: string
  expires_at: string
  invite_url?: string
}

// Alert types
export type AlertType = 'failure_rate' | 'anomalous_latency' | 'new_failure_cluster' | 'error_spike'

export interface AlertRule {
  id: string
  name: string
  alert_type: AlertType
  threshold: number
  window_minutes: number
  cooldown_minutes: number
  notify_roles: string[]
  enabled: boolean
  created_at: string
  last_triggered_at?: string
}

export interface AlertEvent {
  id: string
  alert_rule_id: string
  rule_name: string
  alert_type: AlertType
  triggered_at: string
  value: number
  threshold: number
}

// System Prompt types
export interface SystemPromptListItem {
  id: string
  short_uid: string
  content_preview: string
  content_length: number
  span_count: number
  session_count: number
  created_at: string
  last_seen_at?: string
}

export interface SystemPromptDetail {
  id: string
  short_uid: string
  content: string
  span_count: number
  session_count: number
  created_at: string
  last_seen_at?: string
}

// Auth types
export interface LoginResponse {
  expires_at: string
}

export interface RegisterResponse {
  user_id: string
  email: string
  email_sent?: boolean
  verification_url?: string
  auto_login?: boolean
}

export interface SetupStatusResponse {
  setup_complete: boolean
}

// Pagination
export interface PaginatedResponse<T> {
  data: T[]
  next_cursor?: string
}

// Error envelope
export interface APIError {
  error: { code: string; message: string }
}

// WebSocket event types
export interface WSMessage {
  identifier: { channel: string; org_id?: string; session_id?: string }
  type: string
  payload?: Record<string, unknown>
}
