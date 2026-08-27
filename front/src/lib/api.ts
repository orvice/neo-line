import { createClient, Code, ConnectError } from "@connectrpc/connect"
import { createGrpcWebTransport } from "@connectrpc/connect-web"
import type { Interceptor } from "@connectrpc/connect"
import type { MessageInitShape } from "@bufbuild/protobuf"
import type { Timestamp } from "@bufbuild/protobuf/wkt"
import { timestampDate, timestampFromDate } from "@bufbuild/protobuf/wkt"

import { AuthService } from "@/gen/neoline/v1/auth_pb"
import { SettingsService, type Settings as PbSettings } from "@/gen/neoline/v1/settings_pb"
import { StatusService } from "@/gen/neoline/v1/status_pb"
import { McpTokenService } from "@/gen/neoline/v1/mcp_token_pb"
import { SshService } from "@/gen/neoline/v1/ssh_pb"
import {
  AuditLogService,
  type AuditLog as PbAuditLog,
} from "@/gen/neoline/v1/audit_log_pb"
import {
  ServerService,
  type Server as PbServer,
  type ServerSSH as PbServerSSH,
  type ServerEvent as PbServerEvent,
  type ServerHealth as PbServerHealth,
} from "@/gen/neoline/v1/server_pb"
import {
  MonitorService,
  type Monitor as PbMonitor,
  type CertificateInfo as PbCertificateInfo,
  type CheckResult as PbCheckResult,
  type MonitorUptime as PbMonitorUptime,
} from "@/gen/neoline/v1/monitor_pb"
import {
  MonitorGroupService,
  type MonitorGroup as PbMonitorGroup,
  type AlertPolicy as PbAlertPolicy,
} from "@/gen/neoline/v1/monitor_group_pb"
import {
  NotifyGroupService,
  type NotifyGroup as PbNotifyGroup,
  type AlertChannel as PbAlertChannel,
} from "@/gen/neoline/v1/notify_group_pb"
import {
  DNSProviderAccountService,
  type DNSProviderAccount as PbDNSProviderAccount,
} from "@/gen/neoline/v1/dns_provider_account_pb"
import {
  CertificateIssuerService,
  CertificateIssuerRegistrationStatus as PbIssuerStatus,
  type CertificateIssuer as PbCertificateIssuer,
  type CertificateIssuerDirectoryPreview as PbIssuerPreview,
} from "@/gen/neoline/v1/certificate_issuer_pb"
import {
  ManagedCertificateService,
  CertificateKeyType as PbCertKeyType,
  CertificateValidity as PbCertValidity,
  CertificateOperationType as PbCertOpType,
  CertificateOperationStatus as PbCertOpStatus,
  type ManagedCertificate as PbManagedCertificate,
  type CertificateVersionMetadata as PbCertificateVersionMetadata,
  type CertificateBundle as PbCertificateBundle,
  type CertificateOperation as PbCertificateOperation,
  type IssueConfigSnapshot as PbIssueConfigSnapshot,
} from "@/gen/neoline/v1/managed_certificate_pb"
import {
  type StatusGroup as PbStatusGroup,
  type StatusServer as PbStatusServer,
  type StatusMonitor as PbStatusMonitor,
  type PublicCertificate as PbPublicCertificate,
} from "@/gen/neoline/v1/status_pb"

import type {
  AlertChannel,
  AlertPolicy,
  AuditLog,
  AuditLogQuery,
  CertificateInfo,
  CheckResult,
  CreateMcpTokenResponse,
  Heartbeat,
  LoginResponse,
  McpToken,
  Monitor,
  MonitorGroup,
  MonitorUptime,
  NotifyGroup,
  DNSProviderAccount,
  CertificateIssuer,
  CertificateIssuerDirectoryPreview,
  CertificateIssuerRegistrationStatus,
  CertificateKeyType,
  CertificateOperation,
  CertificateOperationStatus,
  CertificateOperationType,
  CertificateValidity,
  CertificateVersionMetadata,
  CertificateBundle,
  ManagedCertificate,
  IssueConfigSnapshot,
  Server,
  ServerEvent,
  ServerHealth,
  Settings,
  SshExecResponse,
  SshTestConnectionResponse,
  StatusCertificate,
  StatusGroup,
  StatusMonitor,
  StatusOverview,
  StatusServer,
  UptimeWindow,
  User,
} from "./types"

const TOKEN_KEY = "neo-line.token"

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string | null) {
  if (token) localStorage.setItem(TOKEN_KEY, token)
  else localStorage.removeItem(TOKEN_KEY)
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
    this.name = "ApiError"
  }
}

// Connect transport. gRPC-Web is mounted under /api/grpc on the Go server; the dev
// proxy and production nginx forward that prefix to the backend.
const authInterceptor: Interceptor = (next) => async (req) => {
  const token = getToken()
  if (token) req.header.set("Authorization", `Bearer ${token}`)
  return next(req)
}

const transport = createGrpcWebTransport({
  baseUrl: "/api/grpc",
  interceptors: [authInterceptor],
})

const authClient = createClient(AuthService, transport)
const settingsClient = createClient(SettingsService, transport)
const statusClient = createClient(StatusService, transport)
const serverClient = createClient(ServerService, transport)
const monitorClient = createClient(MonitorService, transport)
const groupClient = createClient(MonitorGroupService, transport)
const notifyClient = createClient(NotifyGroupService, transport)
const dnsAccountClient = createClient(DNSProviderAccountService, transport)
const issuerClient = createClient(CertificateIssuerService, transport)
const managedCertClient = createClient(ManagedCertificateService, transport)
const mcpClient = createClient(McpTokenService, transport)
const sshClient = createClient(SshService, transport)
const auditClient = createClient(AuditLogService, transport)

function statusFromCode(code: Code): number {
  switch (code) {
    case Code.Unauthenticated:
      return 401
    case Code.PermissionDenied:
      return 403
    case Code.NotFound:
      return 404
    case Code.InvalidArgument:
      return 400
    case Code.AlreadyExists:
      return 409
    default:
      return 500
  }
}

// call normalizes Connect errors into the ApiError shape the UI already handles,
// including the 401 -> token reset behavior the REST client used to provide.
async function call<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn()
  } catch (err) {
    if (err instanceof ConnectError) {
      const status = statusFromCode(err.code)
      if (status === 401) {
        setToken(null)
        throw new ApiError(401, "登录已过期，请重新登录")
      }
      throw new ApiError(status, err.rawMessage || err.message)
    }
    throw err
  }
}

// ---- proto -> snake_case conversion ----

function iso(ts?: Timestamp): string | undefined {
  return ts ? timestampDate(ts).toISOString() : undefined
}

function certFromProto(c?: PbCertificateInfo): CertificateInfo | undefined {
  if (!c) return undefined
  return {
    subject: c.subject || undefined,
    issuer: c.issuer || undefined,
    dns_names: c.dnsNames.length ? c.dnsNames : undefined,
    serial_number: c.serialNumber || undefined,
    not_before: iso(c.notBefore),
    not_after: iso(c.notAfter),
    days_remaining: c.daysRemaining,
  }
}

function sshFromProto(ssh?: PbServerSSH): Server["ssh"] {
  if (!ssh) return undefined
  return {
    enabled: ssh.enabled,
    host: ssh.host || undefined,
    port: ssh.port || undefined,
    user: ssh.user || undefined,
  }
}

function serverFromProto(s: PbServer): Server {
  return {
    id: s.id,
    name: s.name,
    host: s.host,
    environment: s.environment || undefined,
    region: s.region || undefined,
    tags: s.tags.length ? s.tags : undefined,
    sort_order: s.sortOrder,
    enabled: s.enabled,
    health_status: s.healthStatus as Server["health_status"],
    last_status_change_at: iso(s.lastStatusChangeAt),
    last_check_at: iso(s.lastCheckAt),
    created_at: iso(s.createdAt) ?? "",
    updated_at: iso(s.updatedAt) ?? "",
    ssh: sshFromProto(s.ssh),
  }
}

function monitorFromProto(m: PbMonitor): Monitor {
  return {
    id: m.id,
    server_id: m.serverId,
    group_ids: m.groupIds.length ? m.groupIds : undefined,
    name: m.name,
    kind: m.kind as Monitor["kind"],
    enabled: m.enabled,
    host: m.host || undefined,
    port: m.port || undefined,
    url: m.url || undefined,
    method: m.method || undefined,
    path: m.path || undefined,
    headers: Object.keys(m.headers).length ? m.headers : undefined,
    expected_status_codes: m.expectedStatusCodes || undefined,
    tls_verify: m.tlsVerify,
    sni_name: m.sniName || undefined,
    warning_days: m.warningDays || undefined,
    critical_days: m.criticalDays || undefined,
    interval_seconds: m.intervalSeconds,
    timeout_seconds: m.timeoutSeconds,
    retries: m.retries,
    status: m.status as Monitor["status"],
    last_check_at: iso(m.lastCheckAt),
    last_status_change_at: iso(m.lastStatusChangeAt),
    certificate: certFromProto(m.certificate),
    created_at: iso(m.createdAt) ?? "",
    updated_at: iso(m.updatedAt) ?? "",
  }
}

function checkResultFromProto(r: PbCheckResult): CheckResult {
  return {
    id: r.id,
    server_id: r.serverId,
    monitor_id: r.monitorId,
    status: r.status as CheckResult["status"],
    started_at: iso(r.startedAt) ?? "",
    ended_at: iso(r.endedAt) ?? "",
    duration_ms: Number(r.durationMs),
    error_stage: r.errorStage || undefined,
    error_message: r.errorMessage || undefined,
    remote_address: r.remoteAddress || undefined,
    port: r.port || undefined,
    http_status_code: r.httpStatusCode || undefined,
    certificate: certFromProto(r.certificate),
  }
}

function serverEventFromProto(e: PbServerEvent): ServerEvent {
  return {
    id: e.id,
    server_id: e.serverId,
    previous_status: e.previousStatus,
    current_status: e.currentStatus,
    reason: e.reason || undefined,
    occurred_at: iso(e.occurredAt) ?? "",
  }
}

function serverHealthFromProto(h: PbServerHealth): ServerHealth {
  return {
    server_id: h.serverId,
    status: h.status as ServerHealth["status"],
    last_status_change_at: iso(h.lastStatusChangeAt),
    last_check_at: iso(h.lastCheckAt),
    total_monitors: h.totalMonitors,
    healthy_monitors: h.healthyMonitors,
    warning_monitors: h.warningMonitors,
    critical_monitors: h.criticalMonitors,
    down_monitors: h.downMonitors,
    unknown_monitors: h.unknownMonitors,
  }
}

function uptimeFromProto(u?: PbMonitorUptime): MonitorUptime {
  if (!u) return { windows: {}, heartbeats: [] }
  const windows: Record<string, UptimeWindow> = {}
  for (const [key, w] of Object.entries(u.windows)) {
    windows[key] = {
      window_seconds: Number(w.windowSeconds),
      total: w.total,
      up: w.up,
      down: w.down,
      uptime: w.uptime,
      avg_latency_ms: w.avgLatencyMs,
    }
  }
  const heartbeats: Heartbeat[] = u.heartbeats.map((hb) => ({
    status: hb.status as Heartbeat["status"],
    started_at: iso(hb.startedAt) ?? "",
    duration_ms: Number(hb.durationMs),
  }))
  return { windows, heartbeats }
}

function alertPolicyFromProto(p?: PbAlertPolicy): AlertPolicy {
  if (!p) {
    return {
      enabled: false,
      on_down: false,
      on_recover: false,
      on_warning: false,
      on_critical: false,
    }
  }
  return {
    enabled: p.enabled,
    notify_group_ids: p.notifyGroupIds.length ? p.notifyGroupIds : undefined,
    on_down: p.onDown,
    on_recover: p.onRecover,
    on_warning: p.onWarning,
    on_critical: p.onCritical,
    min_interval_seconds: p.minIntervalSeconds || undefined,
  }
}

function monitorGroupFromProto(g: PbMonitorGroup): MonitorGroup {
  return {
    id: g.id,
    name: g.name,
    description: g.description || undefined,
    sort_order: g.sortOrder,
    alert_policy: alertPolicyFromProto(g.alertPolicy),
    created_at: iso(g.createdAt) ?? "",
    updated_at: iso(g.updatedAt) ?? "",
  }
}

function alertChannelFromProto(c: PbAlertChannel): AlertChannel {
  return {
    type: c.type,
    target: c.target,
    extra: Object.keys(c.extra).length ? c.extra : undefined,
  }
}

function notifyGroupFromProto(g: PbNotifyGroup): NotifyGroup {
  return {
    id: g.id,
    name: g.name,
    description: g.description || undefined,
    channels: g.channels.length ? g.channels.map(alertChannelFromProto) : undefined,
    created_at: iso(g.createdAt) ?? "",
    updated_at: iso(g.updatedAt) ?? "",
  }
}

function dnsProviderAccountFromProto(a: PbDNSProviderAccount): DNSProviderAccount {
  return {
    id: a.id,
    name: a.name,
    provider: a.provider,
    propagation_timeout_seconds: a.propagationTimeoutSeconds,
    token_configured: a.tokenConfigured,
    token_last_verified_at: iso(a.tokenLastVerifiedAt),
    created_at: iso(a.createdAt) ?? "",
    updated_at: iso(a.updatedAt) ?? "",
  }
}

function issuerStatusFromProto(s: PbIssuerStatus): CertificateIssuerRegistrationStatus {
  switch (s) {
    case PbIssuerStatus.PENDING:
      return "Pending"
    case PbIssuerStatus.READY:
      return "Ready"
    case PbIssuerStatus.FAILED:
      return "Failed"
    default:
      return "Unspecified"
  }
}

function certificateIssuerFromProto(i: PbCertificateIssuer): CertificateIssuer {
  return {
    id: i.id,
    name: i.name,
    ca_type: i.caType,
    directory_url: i.directoryUrl,
    email: i.email,
    registration_status: issuerStatusFromProto(i.registrationStatus),
    registration_error: i.registrationError || undefined,
    staging_untrusted: i.stagingUntrusted,
    terms_of_service_url: i.termsOfServiceUrl || undefined,
    terms_of_service_agreed_at: iso(i.termsOfServiceAgreedAt),
    account_key_configured: i.accountKeyConfigured,
    eab_configured: i.eabConfigured,
    created_at: iso(i.createdAt) ?? "",
    updated_at: iso(i.updatedAt) ?? "",
  }
}

function issuerPreviewFromProto(p: PbIssuerPreview): CertificateIssuerDirectoryPreview {
  return {
    ca_type: p.caType,
    directory_url: p.directoryUrl,
    terms_of_service_url: p.termsOfServiceUrl || undefined,
    staging_untrusted: p.stagingUntrusted,
    requires_eab: p.requiresEab,
  }
}

function keyTypeFromProto(t: PbCertKeyType): CertificateKeyType {
  switch (t) {
    case PbCertKeyType.EC_P256:
      return "ec_p256"
    case PbCertKeyType.RSA_2048:
      return "rsa_2048"
    default:
      return "unspecified"
  }
}

function keyTypeToProto(t: CertificateKeyType): PbCertKeyType {
  switch (t) {
    case "rsa_2048":
      return PbCertKeyType.RSA_2048
    case "ec_p256":
      return PbCertKeyType.EC_P256
    default:
      return PbCertKeyType.UNSPECIFIED
  }
}

function validityFromProto(v: PbCertValidity): CertificateValidity {
  switch (v) {
    case PbCertValidity.MISSING:
      return "Missing"
    case PbCertValidity.VALID:
      return "Valid"
    case PbCertValidity.RENEWAL_DUE:
      return "RenewalDue"
    case PbCertValidity.EXPIRED:
      return "Expired"
    case PbCertValidity.REVOKED:
      return "Revoked"
    default:
      return "Unspecified"
  }
}

function opTypeFromProto(t: PbCertOpType): CertificateOperationType {
  switch (t) {
    case PbCertOpType.ISSUE:
      return "Issue"
    case PbCertOpType.RENEW:
      return "Renew"
    case PbCertOpType.REVOKE:
      return "Revoke"
    default:
      return "Unspecified"
  }
}

function opStatusFromProto(s: PbCertOpStatus): CertificateOperationStatus {
  switch (s) {
    case PbCertOpStatus.PENDING:
      return "Pending"
    case PbCertOpStatus.RUNNING:
      return "Running"
    case PbCertOpStatus.SUCCEEDED:
      return "Succeeded"
    case PbCertOpStatus.FAILED:
      return "Failed"
    default:
      return "Unspecified"
  }
}

function issueSnapshotFromProto(s: PbIssueConfigSnapshot): IssueConfigSnapshot {
  return {
    domains: [...s.domains],
    certificate_issuer_id: s.certificateIssuerId,
    dns_provider_account_id: s.dnsProviderAccountId,
    key_type: keyTypeFromProto(s.keyType),
  }
}

function certificateOperationFromProto(op: PbCertificateOperation): CertificateOperation {
  return {
    id: op.id,
    managed_certificate_id: op.managedCertificateId,
    type: opTypeFromProto(op.type),
    status: opStatusFromProto(op.status),
    attempt_count: op.attemptCount,
    config_snapshot: op.configSnapshot
      ? issueSnapshotFromProto(op.configSnapshot)
      : undefined,
    error_summary: op.errorSummary || undefined,
    warning: op.warning || undefined,
    started_at: iso(op.startedAt),
    finished_at: iso(op.finishedAt),
    next_attempt_at: iso(op.nextAttemptAt),
    created_at: iso(op.createdAt) ?? "",
    updated_at: iso(op.updatedAt) ?? "",
  }
}

function certificateVersionFromProto(v: PbCertificateVersionMetadata): CertificateVersionMetadata {
  return {
    id: v.id,
    config_snapshot: v.configSnapshot
      ? issueSnapshotFromProto(v.configSnapshot)
      : undefined,
    leaf_fingerprint: v.leafFingerprint,
    serial_number: v.serialNumber,
    issuer_common_name: v.issuerCommonName,
    not_before: iso(v.notBefore),
    not_after: iso(v.notAfter),
    key_type: keyTypeFromProto(v.keyType),
    staging_untrusted: v.stagingUntrusted,
    created_at: iso(v.createdAt),
  }
}

function certificateBundleFromProto(b: PbCertificateBundle): CertificateBundle {
  return {
    managed_certificate_id: b.managedCertificateId,
    version_id: b.versionId,
    domains: [...b.domains],
    key_type: keyTypeFromProto(b.keyType),
    leaf_fingerprint: b.leafFingerprint,
    not_before: iso(b.notBefore),
    not_after: iso(b.notAfter),
    validity: validityFromProto(b.validity),
    staging_untrusted: b.stagingUntrusted,
    fullchain_pem: b.fullchainPem,
    private_key_pem: b.privateKeyPem,
  }
}

function managedCertificateFromProto(c: PbManagedCertificate): ManagedCertificate {
  return {
    id: c.id,
    name: c.name,
    domains: [...c.domains],
    certificate_issuer_id: c.certificateIssuerId,
    dns_provider_account_id: c.dnsProviderAccountId,
    key_type: keyTypeFromProto(c.keyType),
    auto_renew_enabled: c.autoRenewEnabled ?? true,
    renew_before_days: c.renewBeforeDays || 30,
    notify_group_ids: c.notifyGroupIds.length ? [...c.notifyGroupIds] : undefined,
    server_ids: c.serverIds.length ? [...c.serverIds] : undefined,
    active_validity: validityFromProto(c.activeValidity),
    bundle_available: c.bundleAvailable,
    active_version: c.activeVersion
      ? certificateVersionFromProto(c.activeVersion)
      : undefined,
    latest_operation: c.latestOperation
      ? certificateOperationFromProto(c.latestOperation)
      : undefined,
    created_at: iso(c.createdAt) ?? "",
    updated_at: iso(c.updatedAt) ?? "",
  }
}

function managedCertificateToProto(
  body: Partial<ManagedCertificate> & {
    name: string
    domains: string[]
    certificate_issuer_id: string
    dns_provider_account_id: string
  }
): PbManagedCertificate {
  return {
    $typeName: "neoline.v1.ManagedCertificate",
    id: body.id ?? "",
    name: body.name,
    domains: body.domains,
    certificateIssuerId: body.certificate_issuer_id,
    dnsProviderAccountId: body.dns_provider_account_id,
    keyType: keyTypeToProto(body.key_type ?? "ec_p256"),
    autoRenewEnabled: body.auto_renew_enabled ?? true,
    renewBeforeDays: body.renew_before_days ?? 30,
    notifyGroupIds: body.notify_group_ids ?? [],
    serverIds: body.server_ids ?? [],
    activeValidity: PbCertValidity.UNSPECIFIED,
    bundleAvailable: false,
  } as PbManagedCertificate
}

function settingsFromProto(s?: PbSettings): Settings {
  return {
    site_name: s?.siteName ?? "",
    status_page_title: s?.statusPageTitle ?? "",
    updated_at: iso(s?.updatedAt),
  }
}

function mcpTokenFromProto(t: McpTokenPb): McpToken {
  return {
    id: t.id,
    name: t.name,
    prefix: t.prefix,
    created_at: iso(t.createdAt) ?? "",
    last_used_at: iso(t.lastUsedAt),
  }
}

function auditLogFromProto(log: PbAuditLog): AuditLog {
  return {
    id: log.id,
    source: log.source,
    actor_id: log.actorId || undefined,
    actor_email: log.actorEmail || undefined,
    token_prefix: log.tokenPrefix || undefined,
    action: log.action,
    resource_type: log.resourceType || undefined,
    resource_id: log.resourceId || undefined,
    method: log.method || undefined,
    path: log.path || undefined,
    status_code: log.statusCode || undefined,
    success: log.success,
    error: log.error || undefined,
    duration_ms: Number(log.durationMs),
    remote_ip: log.remoteIp || undefined,
    user_agent: log.userAgent || undefined,
    metadata: Object.keys(log.metadata).length ? log.metadata : undefined,
    occurred_at: iso(log.occurredAt) ?? "",
  }
}

function userFromProto(u?: { id: string; email: string; role: string }): User {
  return { id: u?.id ?? "", email: u?.email ?? "", role: u?.role ?? "" }
}

function statusCertFromProto(c?: PbPublicCertificate): StatusCertificate | undefined {
  if (!c) return undefined
  return {
    not_before: iso(c.notBefore),
    not_after: iso(c.notAfter),
    days_remaining: c.daysRemaining,
  }
}

function statusMonitorFromProto(m: PbStatusMonitor): StatusMonitor {
  return {
    id: m.id,
    server_id: m.serverId,
    name: m.name,
    kind: m.kind as StatusMonitor["kind"],
    status: m.status as StatusMonitor["status"],
    interval_seconds: m.intervalSeconds,
    last_check_at: iso(m.lastCheckAt),
    warning_days: m.warningDays || undefined,
    critical_days: m.criticalDays || undefined,
    certificate: statusCertFromProto(m.certificate),
    uptime: uptimeFromProto(m.uptime),
  }
}

function statusServerFromProto(s: PbStatusServer): StatusServer {
  return {
    id: s.id,
    name: s.name,
    environment: s.environment || undefined,
    tags: s.tags.length ? s.tags : undefined,
    monitors: s.monitors.map(statusMonitorFromProto),
  }
}

function statusGroupFromProto(g: PbStatusGroup): StatusGroup {
  return {
    id: g.id,
    name: g.name,
    description: g.description || undefined,
    sort_order: g.sortOrder,
    servers: g.servers.map(statusServerFromProto),
  }
}

// ---- snake_case -> proto request init ----

function serverInit(b: Partial<Server>): MessageInitShape<ServerInitSchema> {
  return {
    name: b.name,
    host: b.host,
    environment: b.environment ?? "",
    region: b.region ?? "",
    tags: b.tags ?? [],
    sortOrder: b.sort_order ?? 0,
    enabled: b.enabled ?? false,
    healthStatus: b.health_status ?? "",
    ssh: b.ssh
      ? {
          enabled: b.ssh.enabled,
          host: b.ssh.host ?? "",
          port: b.ssh.port ?? 0,
          user: b.ssh.user ?? "",
        }
      : undefined,
  }
}

function monitorInit(b: Partial<Monitor>): MessageInitShape<MonitorInitSchema> {
  return {
    name: b.name,
    kind: b.kind,
    enabled: b.enabled ?? false,
    groupIds: b.group_ids ?? [],
    host: b.host ?? "",
    port: b.port ?? 0,
    url: b.url ?? "",
    method: b.method ?? "",
    path: b.path ?? "",
    headers: b.headers ?? {},
    expectedStatusCodes: b.expected_status_codes ?? "",
    tlsVerify: b.tls_verify ?? false,
    sniName: b.sni_name ?? "",
    warningDays: b.warning_days ?? 0,
    criticalDays: b.critical_days ?? 0,
    intervalSeconds: b.interval_seconds ?? 0,
    timeoutSeconds: b.timeout_seconds ?? 0,
    retries: b.retries ?? 0,
  }
}

function monitorGroupInit(
  b: Partial<MonitorGroup>
): MessageInitShape<MonitorGroupInitSchema> {
  return {
    name: b.name,
    description: b.description ?? "",
    sortOrder: b.sort_order ?? 0,
    alertPolicy: b.alert_policy
      ? {
          enabled: b.alert_policy.enabled,
          notifyGroupIds: b.alert_policy.notify_group_ids ?? [],
          onDown: b.alert_policy.on_down,
          onRecover: b.alert_policy.on_recover,
          onWarning: b.alert_policy.on_warning,
          onCritical: b.alert_policy.on_critical,
          minIntervalSeconds: b.alert_policy.min_interval_seconds ?? 0,
        }
      : undefined,
  }
}

function notifyGroupInit(
  b: Partial<NotifyGroup>
): MessageInitShape<NotifyGroupInitSchema> {
  return {
    name: b.name,
    description: b.description ?? "",
    channels: (b.channels ?? []).map((c) => ({
      type: c.type,
      target: c.target,
      extra: c.extra ?? {},
    })),
  }
}

function dnsProviderAccountInit(
  b: Partial<DNSProviderAccount> & { api_token?: string }
): {
  account: MessageInitShape<DNSProviderAccountInitSchema>
  apiToken?: string
} {
  return {
    account: {
      name: b.name,
      provider: b.provider ?? "cloudflare",
      propagationTimeoutSeconds: b.propagation_timeout_seconds ?? 0,
    },
    apiToken: b.api_token,
  }
}

interface ListResponse {
  next_page_token: string
}

export const api = {
  // Auth
  login: (email: string, password: string) =>
    call<LoginResponse>(async () => {
      const res = await authClient.login({ email, password })
      return {
        token: res.token,
        expires_at: iso(res.expiresAt) ?? "",
        user: userFromProto(res.user),
      }
    }),
  me: () =>
    call<{ user: User }>(async () => {
      const res = await authClient.getCurrentUser({})
      return { user: userFromProto(res.user) }
    }),
  logout: () =>
    call<void>(async () => {
      await authClient.logout({})
    }),

  // Public status overview (slim, unauthenticated)
  getStatusOverview: () =>
    call<StatusOverview>(async () => {
      const res = await statusClient.getStatusOverview({})
      return { groups: res.groups.map(statusGroupFromProto) }
    }),

  // Settings
  getSettings: () =>
    call<{ settings: Settings }>(async () => {
      const res = await settingsClient.getSettings({})
      return { settings: settingsFromProto(res.settings) }
    }),
  updateSettings: (body: Partial<Settings>) =>
    call<{ settings: Settings }>(async () => {
      const res = await settingsClient.updateSettings({
        settings: {
          siteName: body.site_name ?? "",
          statusPageTitle: body.status_page_title ?? "",
        },
      })
      return { settings: settingsFromProto(res.settings) }
    }),

  // Servers
  listServers: (query?: {
    environment?: string
    tags?: string
    page_token?: string
    page_size?: number
  }) =>
    call<ListResponse & { servers: Server[] }>(async () => {
      const res = await serverClient.listServers({
        environment: query?.environment ?? "",
        tags: query?.tags ? query.tags.split(",").map((t) => t.trim()).filter(Boolean) : [],
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 0,
      })
      return {
        servers: res.servers.map(serverFromProto),
        next_page_token: res.nextPageToken,
      }
    }),
  getServer: (id: string) =>
    call<{ server: Server }>(async () => {
      const res = await serverClient.getServer({ id })
      return { server: serverFromProto(res.server!) }
    }),
  createServer: (body: Partial<Server>) =>
    call<{ server: Server }>(async () => {
      const res = await serverClient.createServer({ server: serverInit(body) })
      return { server: serverFromProto(res.server!) }
    }),
  updateServer: (id: string, body: Partial<Server>) =>
    call<{ server: Server }>(async () => {
      const res = await serverClient.updateServer({ id, server: serverInit(body) })
      return { server: serverFromProto(res.server!) }
    }),
  deleteServer: (id: string) =>
    call<void>(async () => {
      await serverClient.deleteServer({ id })
    }),
  getServerHealth: (id: string) =>
    call<{ health: ServerHealth }>(async () => {
      const res = await serverClient.getServerHealth({ id })
      return { health: serverHealthFromProto(res.health!) }
    }),
  listServerEvents: (id: string, query?: { page_token?: string; page_size?: number }) =>
    call<ListResponse & { events: ServerEvent[] }>(async () => {
      const res = await serverClient.listServerEvents({
        id,
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 0,
      })
      return {
        events: res.events.map(serverEventFromProto),
        next_page_token: res.nextPageToken,
      }
    }),
  sshExec: (serverId: string, command: string, timeoutSeconds?: number) =>
    call<SshExecResponse>(async () => {
      const res = await sshClient.exec({
        serverId,
        command,
        timeoutSeconds: timeoutSeconds ?? 0,
      })
      return {
        server_id: res.serverId,
        host: res.host,
        stdout: res.stdout,
        stderr: res.stderr,
        exit_code: res.exitCode,
      }
    }),
  sshTestConnection: (serverId: string) =>
    call<SshTestConnectionResponse>(async () => {
      const res = await sshClient.testConnection({ serverId })
      return {
        server_id: res.serverId,
        host: res.host,
        ok: res.ok,
        output: res.output || undefined,
      }
    }),

  // Monitors
  listMonitors: (serverId: string, query?: { page_token?: string; page_size?: number }) =>
    call<ListResponse & { monitors: Monitor[] }>(async () => {
      const res = await monitorClient.listMonitors({
        serverId,
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 0,
      })
      return {
        monitors: res.monitors.map(monitorFromProto),
        next_page_token: res.nextPageToken,
      }
    }),
  getMonitor: (serverId: string, monitorId: string) =>
    call<{ monitor: Monitor }>(async () => {
      const res = await monitorClient.getMonitor({ serverId, monitorId })
      return { monitor: monitorFromProto(res.monitor!) }
    }),
  createMonitor: (serverId: string, body: Partial<Monitor>) =>
    call<{ monitor: Monitor }>(async () => {
      const res = await monitorClient.createMonitor({
        serverId,
        monitor: monitorInit(body),
      })
      return { monitor: monitorFromProto(res.monitor!) }
    }),
  updateMonitor: (serverId: string, monitorId: string, body: Partial<Monitor>) =>
    call<{ monitor: Monitor }>(async () => {
      const res = await monitorClient.updateMonitor({
        serverId,
        monitorId,
        monitor: monitorInit(body),
      })
      return { monitor: monitorFromProto(res.monitor!) }
    }),
  deleteMonitor: (serverId: string, monitorId: string) =>
    call<void>(async () => {
      await monitorClient.deleteMonitor({ serverId, monitorId })
    }),

  getMonitorUptime: (serverId: string, monitorId: string) =>
    call<{ uptime: MonitorUptime }>(async () => {
      const res = await monitorClient.getMonitorUptime({ serverId, monitorId })
      return { uptime: uptimeFromProto(res.uptime) }
    }),

  // Monitor groups
  listMonitorGroups: (query?: { page_token?: string; page_size?: number }) =>
    call<ListResponse & { groups: MonitorGroup[] }>(async () => {
      const res = await groupClient.listMonitorGroups({
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 0,
      })
      return {
        groups: res.groups.map(monitorGroupFromProto),
        next_page_token: res.nextPageToken,
      }
    }),
  getMonitorGroup: (groupId: string) =>
    call<{ group: MonitorGroup }>(async () => {
      const res = await groupClient.getMonitorGroup({ groupId })
      return { group: monitorGroupFromProto(res.group!) }
    }),
  createMonitorGroup: (body: Partial<MonitorGroup>) =>
    call<{ group: MonitorGroup }>(async () => {
      const res = await groupClient.createMonitorGroup({ group: monitorGroupInit(body) })
      return { group: monitorGroupFromProto(res.group!) }
    }),
  updateMonitorGroup: (groupId: string, body: Partial<MonitorGroup>) =>
    call<{ group: MonitorGroup }>(async () => {
      const res = await groupClient.updateMonitorGroup({
        groupId,
        group: monitorGroupInit(body),
      })
      return { group: monitorGroupFromProto(res.group!) }
    }),
  deleteMonitorGroup: (groupId: string) =>
    call<void>(async () => {
      await groupClient.deleteMonitorGroup({ groupId })
    }),
  listMonitorsByGroup: (groupId: string, query?: { page_token?: string; page_size?: number }) =>
    call<ListResponse & { monitors: Monitor[] }>(async () => {
      const res = await groupClient.listMonitorsByGroup({
        groupId,
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 0,
      })
      return {
        monitors: res.monitors.map(monitorFromProto),
        next_page_token: res.nextPageToken,
      }
    }),

  // Notify groups
  listNotifyGroups: (query?: { page_token?: string; page_size?: number }) =>
    call<ListResponse & { groups: NotifyGroup[] }>(async () => {
      const res = await notifyClient.listNotifyGroups({
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 0,
      })
      return {
        groups: res.groups.map(notifyGroupFromProto),
        next_page_token: res.nextPageToken,
      }
    }),
  getNotifyGroup: (id: string) =>
    call<{ group: NotifyGroup }>(async () => {
      const res = await notifyClient.getNotifyGroup({ notifyGroupId: id })
      return { group: notifyGroupFromProto(res.group!) }
    }),
  createNotifyGroup: (body: Partial<NotifyGroup>) =>
    call<{ group: NotifyGroup }>(async () => {
      const res = await notifyClient.createNotifyGroup({ group: notifyGroupInit(body) })
      return { group: notifyGroupFromProto(res.group!) }
    }),
  updateNotifyGroup: (id: string, body: Partial<NotifyGroup>) =>
    call<{ group: NotifyGroup }>(async () => {
      const res = await notifyClient.updateNotifyGroup({
        notifyGroupId: id,
        group: notifyGroupInit(body),
      })
      return { group: notifyGroupFromProto(res.group!) }
    }),
  deleteNotifyGroup: (id: string) =>
    call<void>(async () => {
      await notifyClient.deleteNotifyGroup({ notifyGroupId: id })
    }),

  // DNS provider accounts
  listDNSProviderAccounts: (query?: { page_token?: string; page_size?: number }) =>
    call<ListResponse & { accounts: DNSProviderAccount[] }>(async () => {
      const res = await dnsAccountClient.listDNSProviderAccounts({
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 0,
      })
      return {
        accounts: res.accounts.map(dnsProviderAccountFromProto),
        next_page_token: res.nextPageToken,
      }
    }),
  getDNSProviderAccount: (id: string) =>
    call<{ account: DNSProviderAccount }>(async () => {
      const res = await dnsAccountClient.getDNSProviderAccount({ dnsProviderAccountId: id })
      return { account: dnsProviderAccountFromProto(res.account!) }
    }),
  createDNSProviderAccount: (
    body: Partial<DNSProviderAccount> & { api_token?: string }
  ) =>
    call<{ account: DNSProviderAccount }>(async () => {
      const init = dnsProviderAccountInit(body)
      const res = await dnsAccountClient.createDNSProviderAccount({
        account: init.account,
        apiToken: init.apiToken ?? "",
      })
      return { account: dnsProviderAccountFromProto(res.account!) }
    }),
  updateDNSProviderAccount: (
    id: string,
    body: Partial<DNSProviderAccount> & { api_token?: string }
  ) =>
    call<{ account: DNSProviderAccount }>(async () => {
      const init = dnsProviderAccountInit(body)
      const res = await dnsAccountClient.updateDNSProviderAccount({
        dnsProviderAccountId: id,
        account: init.account,
        apiToken: init.apiToken ?? "",
      })
      return { account: dnsProviderAccountFromProto(res.account!) }
    }),
  deleteDNSProviderAccount: (id: string) =>
    call<void>(async () => {
      await dnsAccountClient.deleteDNSProviderAccount({ dnsProviderAccountId: id })
    }),

  // Certificate issuers
  listCertificateIssuers: (query?: { page_token?: string; page_size?: number }) =>
    call<ListResponse & { issuers: CertificateIssuer[] }>(async () => {
      const res = await issuerClient.listCertificateIssuers({
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 0,
      })
      return {
        issuers: res.issuers.map(certificateIssuerFromProto),
        next_page_token: res.nextPageToken,
      }
    }),
  getCertificateIssuerDirectoryPreview: (caType: string, customDirectoryUrl?: string) =>
    call<{ preview: CertificateIssuerDirectoryPreview }>(async () => {
      const res = await issuerClient.getCertificateIssuerDirectoryPreview({
        caType,
        customDirectoryUrl: customDirectoryUrl ?? "",
      })
      return { preview: issuerPreviewFromProto(res.preview!) }
    }),
  createCertificateIssuer: (body: {
    name: string
    ca_type: string
    email: string
    custom_directory_url?: string
    account_key_pem?: string
    eab_kid?: string
    eab_hmac?: string
    terms_of_service_agreed: boolean
  }) =>
    call<{ issuer: CertificateIssuer }>(async () => {
      const res = await issuerClient.createCertificateIssuer({
        issuer: { name: body.name, caType: body.ca_type, email: body.email },
        customDirectoryUrl: body.custom_directory_url ?? "",
        accountKeyPem: body.account_key_pem ?? "",
        eabKid: body.eab_kid ?? "",
        eabHmac: body.eab_hmac ?? "",
        termsOfServiceAgreed: body.terms_of_service_agreed,
      })
      return { issuer: certificateIssuerFromProto(res.issuer!) }
    }),
  updateCertificateIssuer: (
    id: string,
    body: {
      name: string
      ca_type?: string
      email?: string
      custom_directory_url?: string
      account_key_pem?: string
      eab_kid?: string
      eab_hmac?: string
    }
  ) =>
    call<{ issuer: CertificateIssuer }>(async () => {
      const res = await issuerClient.updateCertificateIssuer({
        certificateIssuerId: id,
        issuer: {
          name: body.name,
          caType: body.ca_type ?? "",
          email: body.email ?? "",
        },
        customDirectoryUrl: body.custom_directory_url ?? "",
        accountKeyPem: body.account_key_pem ?? "",
        eabKid: body.eab_kid ?? "",
        eabHmac: body.eab_hmac ?? "",
      })
      return { issuer: certificateIssuerFromProto(res.issuer!) }
    }),
  retryCertificateIssuerRegistration: (id: string) =>
    call<{ issuer: CertificateIssuer }>(async () => {
      const res = await issuerClient.retryCertificateIssuerRegistration({
        certificateIssuerId: id,
      })
      return { issuer: certificateIssuerFromProto(res.issuer!) }
    }),
  deleteCertificateIssuer: (id: string) =>
    call<void>(async () => {
      await issuerClient.deleteCertificateIssuer({ certificateIssuerId: id })
    }),

  listManagedCertificates: (query?: { page_token?: string; page_size?: number }) =>
    call<ListResponse & { certificates: ManagedCertificate[] }>(async () => {
      const res = await managedCertClient.listManagedCertificates({
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 50,
      })
      return {
        certificates: res.certificates.map(managedCertificateFromProto),
        next_page_token: res.nextPageToken,
      }
    }),
  getManagedCertificate: (id: string) =>
    call<{ certificate: ManagedCertificate }>(async () => {
      const res = await managedCertClient.getManagedCertificate({
        managedCertificateId: id,
      })
      return { certificate: managedCertificateFromProto(res.certificate!) }
    }),
  createManagedCertificate: (body: {
    name: string
    domains: string[]
    certificate_issuer_id: string
    dns_provider_account_id: string
    key_type?: CertificateKeyType
    auto_renew_enabled?: boolean
    renew_before_days?: number
    notify_group_ids?: string[]
    server_ids?: string[]
  }) =>
    call<{ certificate: ManagedCertificate }>(async () => {
      const res = await managedCertClient.createManagedCertificate({
        certificate: managedCertificateToProto(body),
      })
      return { certificate: managedCertificateFromProto(res.certificate!) }
    }),
  updateManagedCertificate: (
    id: string,
    body: {
      name: string
      domains: string[]
      certificate_issuer_id: string
      dns_provider_account_id: string
      key_type?: CertificateKeyType
      auto_renew_enabled?: boolean
      renew_before_days?: number
      notify_group_ids?: string[]
      server_ids?: string[]
    }
  ) =>
    call<{ certificate: ManagedCertificate }>(async () => {
      const res = await managedCertClient.updateManagedCertificate({
        managedCertificateId: id,
        certificate: managedCertificateToProto(body),
      })
      return { certificate: managedCertificateFromProto(res.certificate!) }
    }),
  submitIssueOperation: (id: string) =>
    call<{ operation: CertificateOperation }>(async () => {
      const res = await managedCertClient.submitIssueOperation({
        managedCertificateId: id,
      })
      return { operation: certificateOperationFromProto(res.operation!) }
    }),
  getCertificateBundle: (id: string) =>
    call<{ bundle: CertificateBundle }>(async () => {
      const res = await managedCertClient.getCertificateBundle({
        managedCertificateId: id,
      })
      return { bundle: certificateBundleFromProto(res.bundle!) }
    }),

  // MCP tokens
  listMcpTokens: () =>
    call<{ tokens: McpToken[] }>(async () => {
      const res = await mcpClient.listMcpTokens({})
      return { tokens: res.tokens.map(mcpTokenFromProto) }
    }),
  createMcpToken: (name: string) =>
    call<CreateMcpTokenResponse>(async () => {
      const res = await mcpClient.createMcpToken({ name })
      return { token: mcpTokenFromProto(res.token!), secret: res.secret }
    }),
  deleteMcpToken: (id: string) =>
    call<void>(async () => {
      await mcpClient.deleteMcpToken({ tokenId: id })
    }),

  // Audit logs
  listAuditLogs: (query?: AuditLogQuery) =>
    call<ListResponse & { logs: AuditLog[] }>(async () => {
      const res = await auditClient.listAuditLogs({
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 50,
        source: query?.source ?? "",
        action: query?.action ?? "",
        resourceType: query?.resource_type ?? "",
        resourceId: query?.resource_id ?? "",
        actorEmail: query?.actor_email ?? "",
        tokenPrefix: query?.token_prefix ?? "",
        success: query?.success,
        startTime: query?.start_time
          ? timestampFromDate(new Date(query.start_time))
          : undefined,
        endTime: query?.end_time
          ? timestampFromDate(new Date(query.end_time))
          : undefined,
      })
      return {
        logs: res.logs.map(auditLogFromProto),
        next_page_token: res.nextPageToken,
      }
    }),

  // Check results
  listCheckResults: (
    serverId: string,
    monitorId: string,
    query?: { page_token?: string; page_size?: number; start_time?: string; end_time?: string }
  ) =>
    call<ListResponse & { results: CheckResult[] }>(async () => {
      const res = await monitorClient.listCheckResults({
        serverId,
        monitorId,
        pageToken: query?.page_token ?? "",
        pageSize: query?.page_size ?? 0,
        startTime: query?.start_time ? timestampFromDate(new Date(query.start_time)) : undefined,
        endTime: query?.end_time ? timestampFromDate(new Date(query.end_time)) : undefined,
      })
      return {
        results: res.results.map(checkResultFromProto),
        next_page_token: res.nextPageToken,
      }
    }),
}

// Type-only aliases used by the init builders above. They reference the
// generated message types so MessageInitShape resolves to the right field set.
type ServerInitSchema = typeof import("@/gen/neoline/v1/server_pb").ServerSchema
type MonitorInitSchema = typeof import("@/gen/neoline/v1/monitor_pb").MonitorSchema
type MonitorGroupInitSchema = typeof import("@/gen/neoline/v1/monitor_group_pb").MonitorGroupSchema
type NotifyGroupInitSchema = typeof import("@/gen/neoline/v1/notify_group_pb").NotifyGroupSchema
type DNSProviderAccountInitSchema = typeof import("@/gen/neoline/v1/dns_provider_account_pb").DNSProviderAccountSchema
type McpTokenPb = import("@/gen/neoline/v1/mcp_token_pb").McpToken
