import { useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { useQueries, useQuery } from "@tanstack/react-query"
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  Clock3,
  Database,
  Gauge,
  Globe2,
  LockKeyhole,
  RefreshCw,
  Search,
  Server as ServerIcon,
  ShieldCheck,
  Terminal,
  Wifi,
  XCircle,
} from "lucide-react"

import { api } from "@/lib/api"
import type {
  CertificateInfo,
  HealthStatus,
  Heartbeat,
  Monitor,
  MonitorUptime,
  Server,
  UptimeWindow,
} from "@/lib/types"
import { useSettings } from "@/lib/settings"
import {
  formatCertExpiry,
  formatRelative,
  formatTime,
  monitorKindLabels,
  statusLabels,
} from "@/lib/format"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

const STATUS_RANK: Record<HealthStatus, number> = {
  Down: 4,
  Critical: 3,
  Warning: 2,
  Unknown: 1,
  Healthy: 0,
}

const STATUS_TONES: Record<
  HealthStatus,
  {
    icon: typeof CheckCircle2
    dot: string
    bar: string
    text: string
    border: string
    bg: string
    softBg: string
    ring: string
    code: string
  }
> = {
  Healthy: {
    icon: CheckCircle2,
    dot: "bg-[#4edea3]",
    bar: "bg-[#4edea3]",
    text: "text-[#4edea3]",
    border: "border-[#4edea3]/30",
    bg: "bg-[#4edea3]/10",
    softBg: "bg-[#4edea3]/5",
    ring: "#4edea3",
    code: "UP",
  },
  Warning: {
    icon: AlertTriangle,
    dot: "bg-[#f59e0b]",
    bar: "bg-[#f59e0b]",
    text: "text-[#ffc174]",
    border: "border-[#f59e0b]/35",
    bg: "bg-[#f59e0b]/12",
    softBg: "bg-[#f59e0b]/6",
    ring: "#f59e0b",
    code: "WARN",
  },
  Critical: {
    icon: AlertTriangle,
    dot: "bg-[#fb7185]",
    bar: "bg-[#fb7185]",
    text: "text-[#fb7185]",
    border: "border-[#fb7185]/35",
    bg: "bg-[#fb7185]/12",
    softBg: "bg-[#fb7185]/6",
    ring: "#fb7185",
    code: "CRIT",
  },
  Down: {
    icon: XCircle,
    dot: "bg-[#ff6b6b]",
    bar: "bg-[#ff6b6b]",
    text: "text-[#ffb4ab]",
    border: "border-[#ff6b6b]/40",
    bg: "bg-[#ff6b6b]/12",
    softBg: "bg-[#ff6b6b]/6",
    ring: "#ff6b6b",
    code: "DOWN",
  },
  Unknown: {
    icon: CircleDashed,
    dot: "bg-[#87929a]",
    bar: "bg-[#64748b]",
    text: "text-[#bdc8d1]",
    border: "border-[#87929a]/30",
    bg: "bg-[#87929a]/10",
    softBg: "bg-[#87929a]/5",
    ring: "#87929a",
    code: "UNK",
  },
}

function normalizeStatus(status: string): HealthStatus {
  return status in STATUS_RANK ? (status as HealthStatus) : "Unknown"
}

function worst(statuses: HealthStatus[]): HealthStatus {
  return statuses.reduce<HealthStatus>(
    (acc, s) => (STATUS_RANK[s] > STATUS_RANK[acc] ? s : acc),
    "Healthy"
  )
}

function overallTitle(status: HealthStatus, total: number): string {
  if (total === 0) return "等待监控数据"
  if (status === "Down" || status === "Critical") return "部分系统发生中断"
  if (status === "Warning") return "部分系统性能降级"
  if (status === "Unknown") return "部分系统状态未知"
  return "所有系统运行正常"
}

function uptimePct(uptime?: MonitorUptime): string {
  const win = uptime?.windows?.["24h"]
  if (!win || win.total === 0) return "-"
  return `${(win.uptime * 100).toFixed(2)}%`
}

function uptimeValue(uptime?: MonitorUptime): number | undefined {
  const win = uptime?.windows?.["24h"]
  if (!win || win.total === 0) return undefined
  return win.uptime * 100
}

function formatUptimeAverage(values: Array<number | undefined>): string {
  const valid = values.filter((value): value is number => value !== undefined)
  if (valid.length === 0) return "-"
  const avg = valid.reduce((sum, value) => sum + value, 0) / valid.length
  return `${avg.toFixed(2)}%`
}

function formatLatency(window?: UptimeWindow): string {
  if (!window || window.total === 0 || window.avg_latency_ms <= 0) return "-"
  if (window.avg_latency_ms >= 1000) {
    return `${(window.avg_latency_ms / 1000).toFixed(2)}s`
  }
  return `${Math.round(window.avg_latency_ms)}ms`
}

function formatSeconds(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`
  return `${(seconds / 3600).toFixed(seconds % 3600 === 0 ? 0 : 1)}h`
}

function statusCountText(count: number): string {
  return count === 0 ? "0" : String(count).padStart(2, "0")
}

export function StatusPage() {
  const settings = useSettings()
  const [searchTerm, setSearchTerm] = useState("")

  const groupsQuery = useQuery({
    queryKey: ["status-groups"],
    queryFn: () => api.listMonitorGroups({ page_size: 200 }),
    refetchInterval: 60_000,
  })
  const groups = groupsQuery.data?.groups ?? []

  const serversQuery = useQuery({
    queryKey: ["status-servers"],
    queryFn: () => api.listServers({ page_size: 200 }),
    refetchInterval: 60_000,
  })
  const serverMap = useMemo(() => {
    const map = new Map<string, Server>()
    for (const s of serversQuery.data?.servers ?? []) map.set(s.id, s)
    return map
  }, [serversQuery.data])

  const monitorQueries = useQueries({
    queries: groups.map((g) => ({
      queryKey: ["status-group-monitors", g.id],
      queryFn: () => api.listMonitorsByGroup(g.id, { page_size: 200 }),
      refetchInterval: 60_000,
    })),
  })

  const sections = useMemo(
    () =>
      groups.map((group, i) => ({
        group,
        monitors: (monitorQueries[i]?.data?.monitors ?? []).filter(
          (m) => m.enabled
        ),
      })),
    [groups, monitorQueries]
  )

  const allMonitors = useMemo(
    () => sections.flatMap((s) => s.monitors),
    [sections]
  )

  const uptimeQueries = useQueries({
    queries: allMonitors.map((m) => ({
      queryKey: ["status-uptime", m.server_id, m.id],
      queryFn: () => api.getMonitorUptime(m.server_id, m.id),
      refetchInterval: 60_000,
    })),
  })

  const uptimeByMonitor = useMemo(() => {
    const map = new Map<string, MonitorUptime>()
    allMonitors.forEach((m, i) => {
      const data = uptimeQueries[i]?.data?.uptime
      if (data) map.set(m.id, data)
    })
    return map
  }, [allMonitors, uptimeQueries])

  const statusCounts = useMemo(() => {
    const counts: Record<HealthStatus, number> = {
      Healthy: 0,
      Warning: 0,
      Critical: 0,
      Down: 0,
      Unknown: 0,
    }
    for (const monitor of allMonitors) {
      counts[normalizeStatus(monitor.status)] += 1
    }
    return counts
  }, [allMonitors])

  const overallStatus =
    allMonitors.length > 0
      ? worst(allMonitors.map((m) => normalizeStatus(m.status)))
      : "Unknown"
  const overallTone = STATUS_TONES[overallStatus]
  const OverallIcon = overallTone.icon
  const activeIncidents =
    statusCounts.Warning + statusCounts.Critical + statusCounts.Down
  const globalUptime = formatUptimeAverage(
    allMonitors.map((m) => uptimeValue(uptimeByMonitor.get(m.id)))
  )
  const serverCount = new Set(allMonitors.map((m) => m.server_id)).size

  const lastUpdated = useMemo(() => {
    const latest = allMonitors
      .map((m) => m.last_check_at)
      .filter((t): t is string => Boolean(t))
      .map((t) => new Date(t).getTime())
      .filter((time) => !Number.isNaN(time))
      .sort((a, b) => a - b)
      .at(-1)
    return latest ? new Date(latest).toISOString() : undefined
  }, [allMonitors])

  const filteredSections = useMemo(() => {
    const query = searchTerm.trim().toLowerCase()
    if (!query) return sections
    return sections
      .map((section) => {
        const groupMatch = [section.group.name, section.group.description]
          .filter(Boolean)
          .some((value) => value!.toLowerCase().includes(query))

        const monitors = groupMatch
          ? section.monitors
          : section.monitors.filter((monitor) =>
              monitorMatchesSearch(monitor, serverMap, query)
            )

        return { ...section, monitors }
      })
      .filter((section) => section.monitors.length > 0)
  }, [searchTerm, sections, serverMap])

  const loading = groupsQuery.isLoading || serversQuery.isLoading
  const isFetching =
    groupsQuery.isFetching ||
    serversQuery.isFetching ||
    monitorQueries.some((q) => q.isFetching) ||
    uptimeQueries.some((q) => q.isFetching)

  return (
    <div className="relative isolate min-h-[calc(100dvh-3.5rem)] overflow-hidden bg-[#031427] text-[#d3e4fe]">
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.18]"
        style={{
          backgroundImage:
            "linear-gradient(rgba(142,213,255,0.16) 1px, transparent 1px), linear-gradient(90deg, rgba(142,213,255,0.16) 1px, transparent 1px)",
          backgroundSize: "48px 48px",
          maskImage:
            "linear-gradient(to bottom, black 0%, transparent 72%), linear-gradient(to right, transparent 0%, black 18%, black 82%, transparent 100%)",
        }}
      />
      <div className="relative mx-auto flex w-full max-w-[1200px] flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-normal text-[#8ed5ff]">
              <Activity className="size-4" />
              Neo Line Status
            </div>
            <h1 className="mt-2 text-2xl font-semibold tracking-normal text-white sm:text-3xl">
              {settings.status_page_title}
            </h1>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <label className="flex h-10 min-w-0 items-center gap-2 rounded-lg border border-[#3e484f] bg-[#102034]/90 px-3 text-[#bdc8d1] transition focus-within:border-[#8ed5ff]">
              <Search className="size-4 shrink-0" />
              <input
                value={searchTerm}
                onChange={(event) => setSearchTerm(event.target.value)}
                placeholder="搜索服务器或监控"
                className="min-w-0 flex-1 border-0 bg-transparent text-sm text-[#d3e4fe] outline-none placeholder:text-[#87929a]"
              />
            </label>
            <button
              type="button"
              onClick={() => {
                groupsQuery.refetch()
                serversQuery.refetch()
                monitorQueries.forEach((q) => q.refetch())
                uptimeQueries.forEach((q) => q.refetch())
              }}
              className="inline-flex h-10 items-center justify-center gap-2 rounded-lg border border-[#38bdf8]/35 bg-[#38bdf8]/10 px-3 text-sm font-semibold text-[#8ed5ff] transition hover:border-[#8ed5ff] hover:bg-[#38bdf8]/15"
            >
              <RefreshCw className={cn("size-4", isFetching && "animate-spin")} />
              <span>刷新状态</span>
            </button>
          </div>
        </div>

        {loading ? (
          <StatusLoading />
        ) : (
          <>
            <section
              className={cn(
                "relative overflow-hidden rounded-lg border bg-[#1b2b3f]/90 p-6 shadow-[0_24px_70px_rgba(0,15,33,0.34)] sm:p-8",
                overallTone.border
              )}
            >
              <div className="absolute inset-x-0 top-0 h-px bg-[#8ed5ff]/40" />
              <div className="relative flex flex-col items-center gap-4 text-center">
                <div
                  className={cn(
                    "flex size-16 items-center justify-center rounded-lg border",
                    overallTone.border,
                    overallTone.bg
                  )}
                >
                  <OverallIcon className={cn("size-8", overallTone.text)} />
                </div>
                <div>
                  <div
                    className={cn(
                      "mx-auto mb-3 inline-flex items-center gap-2 rounded-full border px-3 py-1 text-xs font-semibold",
                      overallTone.border,
                      overallTone.bg,
                      overallTone.text
                    )}
                  >
                    <span className={cn("size-2 rounded-full", overallTone.dot)} />
                    {statusLabels[overallStatus]}
                  </div>
                  <h2 className="text-3xl font-bold tracking-normal text-white sm:text-4xl">
                    {overallTitle(overallStatus, allMonitors.length)}
                  </h2>
                  <p className="mt-2 flex flex-wrap items-center justify-center gap-2 text-sm text-[#bdc8d1]">
                    <Clock3 className="size-4" />
                    最近更新：{formatRelative(lastUpdated)}
                  </p>
                </div>
              </div>
            </section>

            <section className="grid gap-3 md:grid-cols-3">
              <MetricCard
                label="全局 24h 可用性"
                value={globalUptime}
                tone={globalUptime === "-" ? "neutral" : "good"}
                icon={Gauge}
              />
              <MetricCard
                label="活跃异常"
                value={statusCountText(activeIncidents)}
                tone={activeIncidents === 0 ? "neutral" : "warn"}
                icon={AlertTriangle}
              />
              <MetricCard
                label="启用监控 / 服务器"
                value={`${allMonitors.length} / ${serverCount}`}
                tone="neutral"
                icon={ServerIcon}
              />
            </section>

            <section className="grid gap-3 sm:grid-cols-5">
              {(["Healthy", "Warning", "Critical", "Down", "Unknown"] as const).map(
                (status) => (
                  <StatusCounter
                    key={status}
                    status={status}
                    count={statusCounts[status]}
                  />
                )
              )}
            </section>

            {sections.length === 0 ? (
              <EmptyState text="暂无监控分组，请在分组中添加监控项后展示在状态页。" />
            ) : filteredSections.length === 0 ? (
              <EmptyState text="没有匹配的服务器或监控项。" />
            ) : (
              <div className="flex flex-col gap-8">
                {filteredSections.map(({ group, monitors }) => {
                  const serverRows = groupMonitorsByServer(monitors, serverMap)
                  return (
                    <section key={group.id} className="flex flex-col gap-4">
                      <div className="flex flex-col gap-2 border-b border-[#3e484f] pb-3 sm:flex-row sm:items-end sm:justify-between">
                        <div>
                          <div className="text-xs font-semibold uppercase tracking-normal text-[#8ed5ff]">
                            Monitor Group
                          </div>
                          <h2 className="mt-1 text-xl font-semibold tracking-normal text-white">
                            {group.name}
                          </h2>
                          {group.description && (
                            <p className="mt-1 max-w-2xl text-sm text-[#bdc8d1]">
                              {group.description}
                            </p>
                          )}
                        </div>
                        <Link
                          to={`/monitor-groups/${group.id}`}
                          className="inline-flex items-center text-sm font-medium text-[#8ed5ff] transition hover:text-white"
                        >
                          分组详情
                        </Link>
                      </div>

                      {serverRows.length === 0 ? (
                        <EmptyState text="该分组下暂无启用的监控项。" compact />
                      ) : (
                        <div className="grid gap-4 xl:grid-cols-2">
                          {serverRows.map((row) => (
                            <ServerCard
                              key={row.serverId}
                              row={row}
                              uptimeByMonitor={uptimeByMonitor}
                            />
                          ))}
                        </div>
                      )}
                    </section>
                  )
                })}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

function StatusLoading() {
  return (
    <div className="flex flex-col gap-4">
      <Skeleton className="h-52 rounded-lg bg-[#1b2b3f]" />
      <div className="grid gap-3 md:grid-cols-3">
        <Skeleton className="h-24 rounded-lg bg-[#102034]" />
        <Skeleton className="h-24 rounded-lg bg-[#102034]" />
        <Skeleton className="h-24 rounded-lg bg-[#102034]" />
      </div>
      <div className="grid gap-4 xl:grid-cols-2">
        <Skeleton className="h-72 rounded-lg bg-[#102034]" />
        <Skeleton className="h-72 rounded-lg bg-[#102034]" />
      </div>
    </div>
  )
}

function EmptyState({ text, compact = false }: { text: string; compact?: boolean }) {
  return (
    <div
      className={cn(
        "rounded-lg border border-dashed border-[#3e484f] bg-[#102034]/70 text-center text-sm text-[#bdc8d1]",
        compact ? "px-4 py-6" : "px-6 py-12"
      )}
    >
      {text}
    </div>
  )
}

function MetricCard({
  label,
  value,
  tone,
  icon: Icon,
}: {
  label: string
  value: string
  tone: "good" | "warn" | "neutral"
  icon: typeof Gauge
}) {
  const toneClass =
    tone === "good"
      ? "text-[#4edea3]"
      : tone === "warn"
        ? "text-[#ffc174]"
        : "text-white"
  return (
    <div className="rounded-lg border border-[#3e484f] bg-[#102034] p-4">
      <div className="flex items-center justify-between gap-3 text-xs font-semibold uppercase tracking-normal text-[#bdc8d1]">
        <span>{label}</span>
        <Icon className="size-4 text-[#8ed5ff]" />
      </div>
      <div className={cn("mt-2 font-mono text-2xl font-semibold", toneClass)}>
        {value}
      </div>
    </div>
  )
}

function StatusCounter({ status, count }: { status: HealthStatus; count: number }) {
  const tone = STATUS_TONES[status]
  return (
    <div className="flex items-center justify-between rounded-lg border border-[#3e484f] bg-[#0b1c30] px-3 py-2">
      <div className="flex min-w-0 items-center gap-2">
        <span className={cn("size-2 rounded-full", tone.dot)} />
        <span className="truncate text-sm text-[#bdc8d1]">{statusLabels[status]}</span>
      </div>
      <span className={cn("font-mono text-sm font-semibold", tone.text)}>
        {count}
      </span>
    </div>
  )
}

interface ServerRowData {
  serverId: string
  server?: Server
  monitors: Monitor[]
}

function groupMonitorsByServer(
  monitors: Monitor[],
  serverMap: Map<string, Server>
): ServerRowData[] {
  const order: string[] = []
  const byServer = new Map<string, Monitor[]>()
  for (const m of monitors) {
    if (!byServer.has(m.server_id)) {
      byServer.set(m.server_id, [])
      order.push(m.server_id)
    }
    byServer.get(m.server_id)!.push(m)
  }
  return order.map((serverId) => ({
    serverId,
    server: serverMap.get(serverId),
    monitors: byServer.get(serverId)!,
  }))
}

function ServerCard({
  row,
  uptimeByMonitor,
}: {
  row: ServerRowData
  uptimeByMonitor: Map<string, MonitorUptime>
}) {
  const serverStatus = worst(row.monitors.map((m) => normalizeStatus(m.status)))
  const tone = STATUS_TONES[serverStatus]
  const serverName = row.server?.name ?? row.serverId
  const env = row.server?.environment || row.server?.tags?.[0]
  const serverUptime = formatUptimeAverage(
    row.monitors.map((monitor) => uptimeValue(uptimeByMonitor.get(monitor.id)))
  )

  return (
    <article
      className={cn(
        "overflow-hidden rounded-lg border bg-[#102034] shadow-[0_18px_45px_rgba(0,15,33,0.35)] transition hover:border-[#8ed5ff]/60",
        tone.border
      )}
    >
      <div
        className={cn(
          "h-1 w-full",
          serverStatus === "Healthy" ? "bg-[#4edea3]" : tone.bar
        )}
      />
      <header className="flex flex-col gap-3 border-b border-[#3e484f] bg-[#1b2b3f] p-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-[#3e484f] bg-[#031427] text-[#8ed5ff]">
            <ServerIcon className="size-5" />
          </div>
          <div className="min-w-0">
            <Link
              to={`/servers/${row.serverId}`}
              className="block truncate text-lg font-semibold tracking-normal text-white hover:text-[#8ed5ff]"
            >
              {serverName}
            </Link>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-[#bdc8d1]">
              <span className="font-mono">{row.server?.host ?? row.serverId}</span>
              {env && (
                <span className="rounded border border-[#3e484f] bg-[#26364a] px-1.5 py-0.5 font-mono uppercase text-[#bdc8d1]">
                  {env}
                </span>
              )}
            </div>
          </div>
        </div>
        <div
          className={cn(
            "inline-flex w-fit items-center gap-2 rounded-full border px-2.5 py-1 text-xs font-semibold",
            tone.border,
            tone.bg,
            tone.text
          )}
        >
          <span className={cn("size-2 rounded-full", tone.dot)} />
          {tone.code}
        </div>
      </header>

      <div className="flex flex-col gap-3 p-4">
        <div className="grid grid-cols-2 gap-3">
          <ServerMiniStat label="24h 可用性" value={serverUptime} />
          <ServerMiniStat label="监控项" value={String(row.monitors.length)} />
        </div>
        {row.monitors.map((monitor) => (
          <MonitorPanel
            key={monitor.id}
            monitor={monitor}
            uptime={uptimeByMonitor.get(monitor.id)}
          />
        ))}
      </div>
    </article>
  )
}

function ServerMiniStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-[#3e484f]/80 bg-[#031427] px-3 py-2">
      <div className="text-xs font-semibold uppercase tracking-normal text-[#87929a]">
        {label}
      </div>
      <div className="mt-1 font-mono text-sm font-semibold text-[#d3e4fe]">
        {value}
      </div>
    </div>
  )
}

function MonitorPanel({
  monitor,
  uptime,
}: {
  monitor: Monitor
  uptime?: MonitorUptime
}) {
  const status = normalizeStatus(monitor.status)
  const tone = STATUS_TONES[status]
  const window24h = uptime?.windows?.["24h"]
  const Icon = monitorIcon(monitor)
  const cert = monitor.certificate
  const target = monitorTarget(monitor)

  return (
    <Link
      to={`/servers/${monitor.server_id}/monitors/${monitor.id}`}
      className={cn(
        "group block rounded-lg border bg-[#031427] p-3 transition hover:border-[#8ed5ff]/70",
        status === "Healthy" ? "border-[#3e484f]/70" : tone.border,
        status !== "Healthy" && tone.softBg
      )}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <Icon className={cn("size-4 shrink-0", tone.text)} />
            <span className="truncate text-sm font-medium text-white">
              {monitor.name}
            </span>
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-[#bdc8d1]">
            <span>{monitorKindLabels[monitor.kind] ?? monitor.kind}</span>
            <span className="text-[#3e484f]">/</span>
            <span className="break-all font-mono">{target}</span>
          </div>
        </div>
        <div
          className={cn(
            "inline-flex w-fit items-center gap-1.5 rounded border px-2 py-0.5 font-mono text-[11px] font-semibold",
            tone.border,
            tone.bg,
            tone.text
          )}
        >
          <span className={cn("size-1.5 rounded-full", tone.dot)} />
          {tone.code}
        </div>
      </div>

      <div className="mt-3 grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end">
        <div className="grid grid-cols-2 gap-3">
          <MonitorFact label="响应时间" value={formatLatency(window24h)} />
          <MonitorFact label="24h 可用性" value={uptimePct(uptime)} />
          <MonitorFact label="检查间隔" value={formatSeconds(monitor.interval_seconds)} />
          <MonitorFact label="最近检查" value={formatRelative(monitor.last_check_at)} />
        </div>
        {cert ? (
          <CertificateRing cert={cert} monitor={monitor} />
        ) : (
          <MonitorTargetMeta monitor={monitor} />
        )}
      </div>

      <div className="mt-3">
        <div className="mb-1 text-xs font-semibold uppercase tracking-normal text-[#87929a]">
          最近心跳
        </div>
        <UptimeStrip beats={uptime?.heartbeats ?? []} />
      </div>
    </Link>
  )
}

function MonitorFact({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs font-semibold uppercase tracking-normal text-[#87929a]">
        {label}
      </div>
      <div className="mt-1 truncate font-mono text-sm text-[#d3e4fe]" title={value}>
        {value}
      </div>
    </div>
  )
}

function MonitorTargetMeta({ monitor }: { monitor: Monitor }) {
  const meta =
    monitor.kind === "url"
      ? monitor.method || "GET"
      : monitor.port
        ? `:${monitor.port}`
        : "-"
  return (
    <div className="rounded-lg border border-[#3e484f] bg-[#0b1c30] px-3 py-2 text-right">
      <div className="text-xs font-semibold uppercase tracking-normal text-[#87929a]">
        探测参数
      </div>
      <div className="mt-1 font-mono text-sm text-[#d3e4fe]">{meta}</div>
    </div>
  )
}

function CertificateRing({
  cert,
  monitor,
}: {
  cert: CertificateInfo
  monitor: Monitor
}) {
  const days = cert.days_remaining
  const warningDays = monitor.warning_days || 30
  const criticalDays = monitor.critical_days || 7
  const health =
    days === undefined
      ? "Unknown"
      : days < 0 || days <= criticalDays
        ? "Critical"
        : days <= warningDays
          ? "Warning"
          : "Healthy"
  const tone = STATUS_TONES[health]
  const progress =
    days === undefined ? 0 : Math.max(0, Math.min(100, (days / warningDays) * 100))

  return (
    <div className="flex items-center justify-end gap-3 rounded-lg border border-[#3e484f] bg-[#0b1c30] px-3 py-2">
      <div
        className="grid size-12 shrink-0 place-items-center rounded-full"
        style={{
          background: `conic-gradient(${tone.ring} ${progress}%, rgba(62,72,79,0.78) 0)`,
        }}
      >
        <div className="grid size-9 place-items-center rounded-full bg-[#031427]">
          <LockKeyhole className={cn("size-4", tone.text)} />
        </div>
      </div>
      <div className="min-w-0 text-right">
        <div className="text-xs font-semibold uppercase tracking-normal text-[#87929a]">
          TLS 证书
        </div>
        <div className={cn("mt-1 truncate font-mono text-sm", tone.text)}>
          {formatCertExpiry(cert)}
        </div>
      </div>
    </div>
  )
}

function UptimeStrip({ beats }: { beats: Heartbeat[] }) {
  const recent = beats.slice(-42)

  if (recent.length === 0) {
    return (
      <div className="h-6 rounded bg-[#0b1c30] px-2 text-xs leading-6 text-[#87929a]">
        暂无心跳数据
      </div>
    )
  }

  return (
    <div
      className="grid h-6 gap-[2px] overflow-hidden rounded bg-[#0b1c30] p-[2px]"
      style={{
        gridTemplateColumns: `repeat(${recent.length}, minmax(0, 1fr))`,
      }}
    >
      {recent.map((beat, i) => {
        const status = normalizeStatus(beat.status)
        const tone = STATUS_TONES[status]
        return (
          <div
            key={`${beat.started_at}-${i}`}
            className={cn("min-w-0 rounded-[2px] transition hover:opacity-75", tone.bar)}
            title={`${statusLabels[status]} · ${formatTime(beat.started_at)} · ${beat.duration_ms} ms`}
          />
        )
      })}
    </div>
  )
}

function monitorIcon(monitor: Monitor) {
  if (monitor.kind === "url") return Globe2
  if (monitor.kind === "tls_port") return ShieldCheck
  if (monitor.port === 3306 || monitor.port === 5432 || monitor.port === 6379) {
    return Database
  }
  if (monitor.kind === "tcp") return Wifi
  return Terminal
}

function monitorTarget(monitor: Monitor): string {
  if (monitor.url) return monitor.url
  const host = monitor.host || monitor.sni_name || "default"
  if (monitor.port) return `${host}:${monitor.port}`
  return host
}

function monitorMatchesSearch(
  monitor: Monitor,
  serverMap: Map<string, Server>,
  query: string
): boolean {
  const server = serverMap.get(monitor.server_id)
  const values = [
    monitor.name,
    monitor.kind,
    monitor.url,
    monitor.host,
    monitor.sni_name,
    monitor.port?.toString(),
    server?.name,
    server?.host,
    server?.environment,
    ...(server?.tags ?? []),
  ]
  return values
    .filter((value): value is string => Boolean(value))
    .some((value) => value.toLowerCase().includes(query))
}
