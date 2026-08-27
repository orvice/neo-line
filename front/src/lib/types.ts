export type HealthStatus =
  | "Healthy"
  | "Warning"
  | "Critical"
  | "Down"
  | "Unknown"

export type MonitorKind = "tcp" | "url" | "tls" | "tls_port" | "tls_certificate"

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
  sort_order: number
  enabled: boolean
  health_status: HealthStatus
  last_status_change_at?: string
  last_check_at?: string
  created_at: string
  updated_at: string
  ssh?: ServerSSH
}

export interface ServerSSH {
  enabled: boolean
  host?: string
  port?: number
  user?: string
}

export interface Monitor {
  id: string
  server_id: string
  group_ids?: string[]
  name: string
  kind: MonitorKind
  enabled: boolean
  host?: string
  port?: number
  url?: string
  method?: string
  path?: string
  headers?: Record<string, string>
  expected_status_codes?: string
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
  certificate?: CertificateInfo
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

export interface SshExecResponse {
  server_id: string
  host: string
  stdout: string
  stderr: string
  exit_code: number
}

export interface SshTestConnectionResponse {
  server_id: string
  host: string
  ok: boolean
  output?: string
}

export interface UptimeWindow {
  window_seconds: number
  total: number
  up: number
  down: number
  uptime: number
  avg_latency_ms: number
}

export interface Heartbeat {
  status: HealthStatus
  started_at: string
  duration_ms: number
}

export interface MonitorUptime {
  windows: Record<string, UptimeWindow>
  heartbeats: Heartbeat[]
}

export interface Paged<T> {
  items: T[]
  next_page_token: string
}

export interface AlertChannel {
  type: string
  target: string
  extra?: Record<string, string>
}

export interface NotifyGroup {
  id: string
  name: string
  description?: string
  channels?: AlertChannel[]
  created_at: string
  updated_at: string
}

export interface DNSProviderAccount {
  id: string
  name: string
  provider: string
  propagation_timeout_seconds: number
  token_configured: boolean
  token_last_verified_at?: string
  created_at: string
  updated_at: string
}

export type CertificateIssuerRegistrationStatus =
  | "Pending"
  | "Ready"
  | "Failed"
  | "Unspecified"

export type CertificateIssuerCAType =
  | "lets_encrypt_production"
  | "lets_encrypt_staging"
  | "zerossl"
  | "google_public_ca"
  | "custom"

export interface CertificateIssuer {
  id: string
  name: string
  ca_type: CertificateIssuerCAType | string
  directory_url: string
  email: string
  registration_status: CertificateIssuerRegistrationStatus
  registration_error?: string
  staging_untrusted: boolean
  terms_of_service_url?: string
  terms_of_service_agreed_at?: string
  account_key_configured: boolean
  eab_configured: boolean
  created_at: string
  updated_at: string
}

export interface CertificateIssuerDirectoryPreview {
  ca_type: string
  directory_url: string
  terms_of_service_url?: string
  staging_untrusted: boolean
  requires_eab: boolean
}

export type CertificateKeyType = "ec_p256" | "rsa_2048" | "unspecified"

export type CertificateValidity =
  | "Missing"
  | "Valid"
  | "RenewalDue"
  | "Expired"
  | "Revoked"
  | "Unspecified"

export type CertificateOperationType = "Issue" | "Renew" | "Revoke" | "Unspecified"

export type CertificateOperationStatus =
  | "Pending"
  | "Running"
  | "Succeeded"
  | "Failed"
  | "Unspecified"

export interface IssueConfigSnapshot {
  domains: string[]
  certificate_issuer_id: string
  dns_provider_account_id: string
  key_type: CertificateKeyType
}

export interface CertificateOperation {
  id: string
  managed_certificate_id: string
  type: CertificateOperationType
  status: CertificateOperationStatus
  attempt_count: number
  config_snapshot?: IssueConfigSnapshot
  error_summary?: string
  warning?: string
  started_at?: string
  finished_at?: string
  next_attempt_at?: string
  created_at: string
  updated_at: string
  target_version_id?: string
  revocation_reason?: CertificateRevocationReason
}

export type CertificateRevocationReason =
  | "unspecified"
  | "key_compromise"
  | "ca_compromise"
  | "affiliation_changed"
  | "superseded"
  | "cessation_of_operation"
  | "certificate_hold"
  | "privilege_withdrawn"
  | "aa_compromise"

export interface CertificateVersionMetadata {
  id: string
  config_snapshot?: IssueConfigSnapshot
  leaf_fingerprint: string
  serial_number: string
  issuer_common_name: string
  not_before?: string
  not_after?: string
  key_type: CertificateKeyType
  staging_untrusted: boolean
  created_at?: string
  revoked_at?: string
  revoke_pending?: boolean
}

export interface CertificateBundle {
  managed_certificate_id: string
  version_id: string
  domains: string[]
  key_type: CertificateKeyType
  leaf_fingerprint: string
  not_before?: string
  not_after?: string
  validity: CertificateValidity
  staging_untrusted: boolean
  fullchain_pem: Uint8Array
  private_key_pem: Uint8Array
}

export interface ManagedCertificate {
  id: string
  name: string
  domains: string[]
  certificate_issuer_id: string
  dns_provider_account_id: string
  key_type: CertificateKeyType
  auto_renew_enabled: boolean
  renew_before_days: number
  effective_renewal_window_days?: number
  next_renewal_at?: string
  notify_group_ids?: string[]
  server_ids?: string[]
  active_validity: CertificateValidity
  bundle_available: boolean
  has_unpublished_desired_changes?: boolean
  active_version?: CertificateVersionMetadata
  previous_version?: CertificateVersionMetadata
  latest_operation?: CertificateOperation
  created_at: string
  updated_at: string
}

export interface AlertPolicy {
  enabled: boolean
  notify_group_ids?: string[]
  on_down: boolean
  on_recover: boolean
  on_warning: boolean
  on_critical: boolean
  min_interval_seconds?: number
}

export interface Settings {
  site_name: string
  status_page_title: string
  updated_at?: string
}

export interface MonitorGroup {
  id: string
  name: string
  description?: string
  sort_order: number
  alert_policy: AlertPolicy
  created_at: string
  updated_at: string
}

export interface McpToken {
  id: string
  name: string
  prefix: string
  created_at: string
  last_used_at?: string
}

export interface CreateMcpTokenResponse {
  token: McpToken
  secret: string
}

export interface CertificateAccessToken {
  id: string
  server_id: string
  name: string
  prefix: string
  created_at: string
  last_used_at?: string
  expires_at?: string
  expired: boolean
}

export interface CreateCertificateAccessTokenResponse {
  token: CertificateAccessToken
  secret: string
}

export interface AuditLog {
  id: string
  source: string
  actor_id?: string
  actor_email?: string
  token_prefix?: string
  action: string
  resource_type?: string
  resource_id?: string
  method?: string
  path?: string
  status_code?: number
  success: boolean
  error?: string
  duration_ms: number
  remote_ip?: string
  user_agent?: string
  metadata?: Record<string, string>
  occurred_at: string
}

export interface AuditLogQuery {
  page_token?: string
  page_size?: number
  source?: string
  action?: string
  resource_type?: string
  resource_id?: string
  actor_email?: string
  token_prefix?: string
  success?: boolean
  start_time?: string
  end_time?: string
}

// Status overview is the slim, public payload backing the anonymous status page.
// It deliberately omits hosts, URLs, ports, headers, and certificate identity.
export interface StatusCertificate {
  not_before?: string
  not_after?: string
  days_remaining?: number
}

export interface StatusMonitor {
  id: string
  server_id: string
  name: string
  kind: MonitorKind
  status: HealthStatus
  interval_seconds: number
  last_check_at?: string
  warning_days?: number
  critical_days?: number
  certificate?: StatusCertificate
  uptime: MonitorUptime
}

export interface StatusServer {
  id: string
  name: string
  environment?: string
  tags?: string[]
  monitors: StatusMonitor[]
}

export interface StatusGroup {
  id: string
  name: string
  description?: string
  sort_order: number
  servers: StatusServer[]
}

export interface StatusOverview {
  groups: StatusGroup[]
}
