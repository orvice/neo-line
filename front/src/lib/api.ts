import type {
  CheckResult,
  LoginResponse,
  Monitor,
  MonitorUptime,
  Server,
  ServerEvent,
  ServerHealth,
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

interface RequestOptions {
  method?: string
  body?: unknown
  query?: Record<string, string | number | undefined | null>
  auth?: boolean
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, query, auth = true } = opts
  const url = new URL(`/v1${path}`, window.location.origin)
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== null && value !== "") {
        url.searchParams.set(key, String(value))
      }
    }
  }

  const headers: Record<string, string> = {}
  if (body !== undefined) headers["Content-Type"] = "application/json"
  if (auth) {
    const token = getToken()
    if (token) headers["Authorization"] = `Bearer ${token}`
  }

  const res = await fetch(url.toString(), {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    setToken(null)
    throw new ApiError(401, "登录已过期，请重新登录")
  }

  if (res.status === 204) {
    return undefined as T
  }

  const text = await res.text()
  const data = text ? JSON.parse(text) : null

  if (!res.ok) {
    const message =
      (data && (data.error || data.detail)) || `请求失败 (${res.status})`
    throw new ApiError(res.status, message)
  }

  return data as T
}

interface ListResponse {
  next_page_token: string
}

export const api = {
  // Auth
  login: (email: string, password: string) =>
    request<LoginResponse>("/auth/login", {
      method: "POST",
      body: { email, password },
      auth: false,
    }),
  me: () => request<{ user: User }>("/auth/me"),
  logout: () => request<void>("/auth/logout", { method: "POST" }),

  // Servers
  listServers: (query?: { environment?: string; tags?: string; page_token?: string; page_size?: number }) =>
    request<ListResponse & { servers: Server[] }>("/servers", { query, auth: false }),
  getServer: (id: string) => request<{ server: Server }>(`/servers/${id}`, { auth: false }),
  createServer: (body: Partial<Server>) =>
    request<{ server: Server }>("/servers", { method: "POST", body }),
  updateServer: (id: string, body: Partial<Server>) =>
    request<{ server: Server }>(`/servers/${id}`, { method: "PUT", body }),
  deleteServer: (id: string) =>
    request<void>(`/servers/${id}`, { method: "DELETE" }),
  getServerHealth: (id: string) =>
    request<{ health: ServerHealth }>(`/servers/${id}/health`, { auth: false }),
  listServerEvents: (id: string, query?: { page_token?: string; page_size?: number }) =>
    request<ListResponse & { events: ServerEvent[] }>(`/servers/${id}/events`, { query, auth: false }),

  // Monitors
  listMonitors: (serverId: string, query?: { page_token?: string; page_size?: number }) =>
    request<ListResponse & { monitors: Monitor[] }>(`/servers/${serverId}/monitors`, { query, auth: false }),
  getMonitor: (serverId: string, monitorId: string) =>
    request<{ monitor: Monitor }>(`/servers/${serverId}/monitors/${monitorId}`, { auth: false }),
  createMonitor: (serverId: string, body: Partial<Monitor>) =>
    request<{ monitor: Monitor }>(`/servers/${serverId}/monitors`, { method: "POST", body }),
  updateMonitor: (serverId: string, monitorId: string, body: Partial<Monitor>) =>
    request<{ monitor: Monitor }>(`/servers/${serverId}/monitors/${monitorId}`, { method: "PUT", body }),
  deleteMonitor: (serverId: string, monitorId: string) =>
    request<void>(`/servers/${serverId}/monitors/${monitorId}`, { method: "DELETE" }),

  getMonitorUptime: (serverId: string, monitorId: string) =>
    request<{ uptime: MonitorUptime }>(
      `/servers/${serverId}/monitors/${monitorId}/uptime`,
      { auth: false }
    ),

  // Check results
  listCheckResults: (
    serverId: string,
    monitorId: string,
    query?: { page_token?: string; page_size?: number; start_time?: string; end_time?: string }
  ) =>
    request<ListResponse & { results: CheckResult[] }>(
      `/servers/${serverId}/monitors/${monitorId}/results`,
      { query, auth: false }
    ),
}
