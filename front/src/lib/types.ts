export type HealthStatus =
  | "Healthy"
  | "Warning"
  | "Critical"
  | "Down"
  | "Unknown"

export type MonitorKind = "tcp" | "url" | "tls_port"

export interface User {
  id: string
  email: string
  role: string
}

export interface LoginResponse {
  token: string
  expires_at: string
  user: User
}

export interface Server {
  id: string
  name: string
  host: string
  environment?: string
  region?: string
  tags?: string[]
  enabled: boolean
  health_status: HealthStatus
  last_status_change_at?: string
  last_check_at?: string
  created_at: string
  updated_at: string
}

export interface Monitor {
  id: string
  server_id: string
  name: string
  kind: MonitorKind
  enabled: boolean
  host?: string
  port?: number
  url?: string
  method?: string
  path?: string
  headers?: Record<string, string>
  expected_status_codes?: number[]
  tls_verify: boolean
  sni_name?: string
  warning_days?: number
  critical_days?: number
  interval_seconds: number
  timeout_seconds: number
  retries: number
  status: HealthStatus
  last_check_at?: string
  last_status_change_at?: string
  created_at: string
  updated_at: string
}

export interface CertificateInfo {
  subject?: string
  issuer?: string
  dns_names?: string[]
  serial_number?: string
  not_before?: string
  not_after?: string
  days_remaining?: number
}

export interface CheckResult {
  id: string
  server_id: string
  monitor_id: string
  status: HealthStatus
  started_at: string
  ended_at: string
  duration_ms: number
  error_stage?: string
  error_message?: string
  remote_address?: string
  port?: number
  http_status_code?: number
  certificate?: CertificateInfo
}

export interface ServerEvent {
  id: string
  server_id: string
  previous_status: string
  current_status: string
  reason?: string
  occurred_at: string
}

export interface ServerHealth {
  server_id: string
  status: HealthStatus
  last_status_change_at?: string
  last_check_at?: string
  total_monitors: number
  healthy_monitors: number
  warning_monitors: number
  critical_monitors: number
  down_monitors: number
  unknown_monitors: number
}

export interface Paged<T> {
  items: T[]
  next_page_token: string
}
