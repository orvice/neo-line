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
  type StatusGroup as PbStatusGroup,
  type StatusServer as PbStatusServer,
  type StatusMonitor as PbStatusMonitor,
  type PublicCertificate as PbPublicCertificate,
} from "@/gen/neoline/v1/status_pb"

import type {
  AlertChannel,
  AlertPolicy,
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
  Server,
  ServerEvent,
  ServerHealth,
  Settings,
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
const mcpClient = createClient(McpTokenService, transport)

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
type McpTokenPb = import("@/gen/neoline/v1/mcp_token_pb").McpToken
