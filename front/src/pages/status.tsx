import { useEffect, useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  Clock3,
  Gauge,
  Globe2,
  LayoutGrid,
  LockKeyhole,
  RefreshCw,
  Rows3,
  Search,
  Server as ServerIcon,
  ShieldCheck,
  Terminal,
  Wifi,
  XCircle,
} from "lucide-react"

import { api } from "@/lib/api"
import type {
  HealthStatus,
  Heartbeat,
  MonitorUptime,
  StatusCertificate,
  StatusGroup,
  StatusMonitor,
  StatusServer,
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
    dot: "bg-emerald-500",
    bar: "bg-emerald-500",
    text: "text-emerald-600 dark:text-emerald-400",
    border: "border-emerald-500/30",
    bg: "bg-emerald-500/10",
    softBg: "bg-emerald-500/5",
    ring: "#10b981",
    code: "UP",
  },
  Warning: {
    icon: AlertTriangle,
    dot: "bg-amber-500",
    bar: "bg-amber-500",
    text: "text-amber-600 dark:text-amber-300",
    border: "border-amber-500/35",
    bg: "bg-amber-500/10",
    softBg: "bg-amber-500/5",
    ring: "#f59e0b",
    code: "WARN",
  },
  Critical: {
    icon: AlertTriangle,
    dot: "bg-orange-500",
    bar: "bg-orange-500",
    text: "text-orange-600 dark:text-orange-400",
    border: "border-orange-500/35",
    bg: "bg-orange-500/10",
    softBg: "bg-orange-500/5",
    ring: "#f97316",
    code: "CRIT",
  },
  Down: {
    icon: XCircle,
    dot: "bg-red-500",
    bar: "bg-red-500",
    text: "text-red-600 dark:text-red-400",
    border: "border-red-500/40",
    bg: "bg-red-500/10",
    softBg: "bg-red-500/5",
    ring: "#ef4444",
    code: "DOWN",
  },
  Unknown: {
    icon: CircleDashed,
    dot: "bg-neutral-400",
    bar: "bg-neutral-500",
    text: "text-muted-foreground",
    border: "border-neutral-500/30",
    bg: "bg-neutral-400/10",
    softBg: "bg-neutral-400/5",
    ring: "#737373",
    code: "UNK",
  },
}

type Density = "comfortable" | "compact"

const DENSITY_STORAGE_KEY = "status-page-density"

function useDensity(): [Density, (value: Density) => void] {
  const [density, setDensity] = useState<Density>(() => {
    if (typeof window === "undefined") return "comfortable"
    return window.localStorage.getItem(DENSITY_STORAGE_KEY) === "compact"
      ? "compact"
      : "comfortable"
  })
  useEffect(() => {
    window.localStorage.setItem(DENSITY_STORAGE_KEY, density)
  }, [density])
  return [density, setDensity]
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
  const [density, setDensity] = useDensity()
  const compact = density === "compact"

  const overviewQuery = useQuery({
    queryKey: ["status-overview"],
    queryFn: () => api.getStatusOverview(),
    refetchInterval: 60_000,
  })
  const groups = useMemo(
    () => overviewQuery.data?.groups ?? [],
    [overviewQuery.data]
  )

  const allMonitors = useMemo(
    () => groups.flatMap((g) => g.servers.flatMap((s) => s.monitors)),
    [groups]
  )

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
    allMonitors.map((m) => uptimeValue(m.uptime))
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

  const filteredGroups = useMemo(
    () => filterGroups(groups, searchTerm),
    [groups, searchTerm]
  )

  const loading = overviewQuery.isLoading
  const isFetching = overviewQuery.isFetching

  return (
    <div className="relative isolate min-h-[calc(100dvh-3.5rem)] overflow-hidden bg-background text-foreground">
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.18]"
        style={{
          backgroundImage:
            "linear-gradient(color-mix(in oklch, var(--color-primary) 18%, transparent) 1px, transparent 1px), linear-gradient(90deg, color-mix(in oklch, var(--color-primary) 18%, transparent) 1px, transparent 1px)",
          backgroundSize: "48px 48px",
          maskImage:
            "linear-gradient(to bottom, black 0%, transparent 72%), linear-gradient(to right, transparent 0%, black 18%, black 82%, transparent 100%)",
        }}
      />
      <div className="relative mx-auto flex w-full max-w-[1200px] flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-normal text-primary">
              <Activity className="size-4" />
              Neo Line Status
            </div>
            <h1 className="mt-2 text-2xl font-semibold tracking-normal text-foreground sm:text-3xl">
              {settings.status_page_title}
            </h1>
          </div>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <label className="flex h-10 min-w-0 items-center gap-2 rounded-lg border border-border bg-card/90 px-3 text-muted-foreground transition focus-within:border-primary">
              <Search className="size-4 shrink-0" />
              <input
                value={searchTerm}
                onChange={(event) => setSearchTerm(event.target.value)}
                placeholder="搜索服务器或监控"
                className="min-w-0 flex-1 border-0 bg-transparent text-sm text-foreground outline-none placeholder:text-muted-foreground"
              />
            </label>
            <div className="flex h-10 items-center rounded-lg border border-border bg-card/90 p-1">
              <button
                type="button"
                onClick={() => setDensity("comfortable")}
                aria-pressed={!compact}
                title="标准布局"
                className={cn(
                  "inline-flex h-full items-center gap-1.5 rounded-md px-2.5 text-sm font-semibold transition",
                  !compact
                    ? "bg-primary/15 text-primary"
                    : "text-muted-foreground hover:text-muted-foreground"
                )}
              >
                <LayoutGrid className="size-4" />
                <span className="hidden sm:inline">标准</span>
              </button>
              <button
                type="button"
                onClick={() => setDensity("compact")}
                aria-pressed={compact}
                title="紧凑布局"
                className={cn(
                  "inline-flex h-full items-center gap-1.5 rounded-md px-2.5 text-sm font-semibold transition",
                  compact
                    ? "bg-primary/15 text-primary"
                    : "text-muted-foreground hover:text-muted-foreground"
                )}
              >
                <Rows3 className="size-4" />
                <span className="hidden sm:inline">紧凑</span>
              </button>
            </div>
            <button
              type="button"
              onClick={() => overviewQuery.refetch()}
              className="inline-flex h-10 items-center justify-center gap-2 rounded-lg border border-primary/35 bg-primary/10 px-3 text-sm font-semibold text-primary transition hover:border-primary hover:bg-primary/15"
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
                "relative overflow-hidden rounded-lg border bg-card p-6 shadow-[0_24px_70px_rgba(0,0,0,0.12)] sm:p-8 dark:bg-surface-recessed",
                overallTone.border
              )}
            >
              <div className="absolute inset-x-0 top-0 h-px bg-primary/40" />
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
                  <h2 className="text-3xl font-bold tracking-normal text-foreground sm:text-4xl">
                    {overallTitle(overallStatus, allMonitors.length)}
                  </h2>
                  <p className="mt-2 flex flex-wrap items-center justify-center gap-2 text-sm text-muted-foreground">
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

            {groups.length === 0 ? (
              <EmptyState text="暂无监控分组，请在分组中添加监控项后展示在状态页。" />
            ) : filteredGroups.length === 0 ? (
              <EmptyState text="没有匹配的服务器或监控项。" />
            ) : (
              <div className="flex flex-col gap-8">
                {filteredGroups.map((group) => (
                  <section key={group.id} className="flex flex-col gap-4">
                    <div className="flex flex-col gap-2 border-b border-border pb-3 sm:flex-row sm:items-end sm:justify-between">
                      <div>
                        <div className="text-xs font-semibold uppercase tracking-normal text-primary">
                          Monitor Group
                        </div>
                        <h2 className="mt-1 text-xl font-semibold tracking-normal text-foreground">
                          {group.name}
                        </h2>
                        {group.description && (
                          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
                            {group.description}
                          </p>
                        )}
                      </div>
                      <Link
                        to={`/monitor-groups/${group.id}`}
                        className="inline-flex items-center text-sm font-medium text-primary transition hover:text-foreground"
                      >
                        分组详情
                      </Link>
                    </div>

                    {group.servers.length === 0 ? (
                      <EmptyState text="该分组下暂无启用的监控项。" compact />
                    ) : (
                      <div
                        className={cn(
                          "grid",
                          compact
                            ? "gap-3 sm:grid-cols-2 xl:grid-cols-3"
                            : "gap-4 xl:grid-cols-2"
                        )}
                      >
                        {group.servers.map((server) => (
                          <ServerCard
                            key={server.id}
                            server={server}
                            compact={compact}
                          />
                        ))}
                      </div>
                    )}
                  </section>
                ))}
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
      <Skeleton className="h-52 rounded-lg bg-surface-recessed" />
      <div className="grid gap-3 md:grid-cols-3">
        <Skeleton className="h-24 rounded-lg bg-card" />
        <Skeleton className="h-24 rounded-lg bg-card" />
        <Skeleton className="h-24 rounded-lg bg-card" />
      </div>
      <div className="grid gap-4 xl:grid-cols-2">
        <Skeleton className="h-72 rounded-lg bg-card" />
        <Skeleton className="h-72 rounded-lg bg-card" />
      </div>
    </div>
  )
}

function EmptyState({ text, compact = false }: { text: string; compact?: boolean }) {
  return (
    <div
      className={cn(
        "rounded-lg border border-dashed border-border bg-card/70 text-center text-sm text-muted-foreground",
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
      ? "text-emerald-600 dark:text-emerald-400"
      : tone === "warn"
        ? "text-amber-600 dark:text-amber-300"
        : "text-foreground"
  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-center justify-between gap-3 text-xs font-semibold uppercase tracking-normal text-muted-foreground">
        <span>{label}</span>
        <Icon className="size-4 text-primary" />
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
    <div className="flex items-center justify-between rounded-lg border border-border bg-surface-elevated px-3 py-2">
      <div className="flex min-w-0 items-center gap-2">
        <span className={cn("size-2 rounded-full", tone.dot)} />
        <span className="truncate text-sm text-muted-foreground">{statusLabels[status]}</span>
      </div>
      <span className={cn("font-mono text-sm font-semibold", tone.text)}>
        {count}
      </span>
    </div>
  )
}

function ServerCard({
  server,
  compact = false,
}: {
  server: StatusServer
  compact?: boolean
}) {
  const serverStatus = worst(server.monitors.map((m) => normalizeStatus(m.status)))
  const tone = STATUS_TONES[serverStatus]
  const serverName = server.name || server.id
  const env = server.environment || server.tags?.[0]
  const serverUptime = formatUptimeAverage(
    server.monitors.map((monitor) => uptimeValue(monitor.uptime))
  )

  return (
    <article
      className={cn(
        "overflow-hidden rounded-lg border bg-card shadow-[0_18px_45px_rgba(0,0,0,0.12)] transition hover:border-primary/60",
        tone.border
      )}
    >
      <div
        className={cn(
          "h-1 w-full",
          serverStatus === "Healthy" ? "bg-emerald-500" : tone.bar
        )}
      />
      <header
        className={cn(
          "flex flex-col gap-3 border-b border-border bg-surface-recessed sm:flex-row sm:items-center sm:justify-between",
          compact ? "p-3" : "p-4"
        )}
      >
        <div className="flex min-w-0 items-center gap-3">
          <div
            className={cn(
              "flex shrink-0 items-center justify-center rounded-lg border border-border bg-background text-primary",
              compact ? "size-8" : "size-9"
            )}
          >
            <ServerIcon className={compact ? "size-4" : "size-5"} />
          </div>
          <div className="min-w-0">
            <Link
              to={`/servers/${server.id}`}
              className={cn(
                "block truncate font-semibold tracking-normal text-foreground hover:text-primary",
                compact ? "text-base" : "text-lg"
              )}
            >
              {serverName}
            </Link>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span>{server.monitors.length} 个监控</span>
              {env && (
                <span className="rounded border border-border bg-accent px-1.5 py-0.5 font-mono uppercase text-muted-foreground">
                  {env}
                </span>
              )}
            </div>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {compact && (
            <span className="font-mono text-xs font-semibold text-muted-foreground">
              {serverUptime}
            </span>
          )}
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
        </div>
      </header>

      <div className={cn("flex flex-col", compact ? "gap-2 p-3" : "gap-3 p-4")}>
        {!compact && (
          <div className="grid grid-cols-2 gap-3">
            <ServerMiniStat label="24h 可用性" value={serverUptime} />
            <ServerMiniStat label="监控项" value={String(server.monitors.length)} />
          </div>
        )}
        {server.monitors.map((monitor) =>
          compact ? (
            <CompactMonitorRow key={monitor.id} monitor={monitor} />
          ) : (
            <MonitorPanel key={monitor.id} monitor={monitor} />
          )
        )}
      </div>
    </article>
  )
}

function CompactMonitorRow({ monitor }: { monitor: StatusMonitor }) {
  const status = normalizeStatus(monitor.status)
  const tone = STATUS_TONES[status]
  const window24h = monitor.uptime?.windows?.["24h"]
  const Icon = monitorIcon(monitor)

  return (
    <Link
      to={`/servers/${monitor.server_id}/monitors/${monitor.id}`}
      className={cn(
        "group flex items-center gap-3 rounded-md border bg-background px-2.5 py-2 transition hover:border-primary/70",
        status === "Healthy" ? "border-border/70" : tone.border,
        status !== "Healthy" && tone.softBg
      )}
    >
      <span className={cn("size-2 shrink-0 rounded-full", tone.dot)} />
      <Icon className={cn("size-4 shrink-0", tone.text)} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium text-foreground">
          {monitor.name}
        </div>
        <div className="truncate font-mono text-xs text-muted-foreground">
          {monitorKindLabels[monitor.kind] ?? monitor.kind}
        </div>
      </div>
      <div className="hidden w-24 shrink-0 sm:block">
        <CompactHeartbeats beats={monitor.uptime?.heartbeats ?? []} />
      </div>
      <div className="w-16 shrink-0 text-right">
        <div className="font-mono text-sm text-foreground">
          {uptimePct(monitor.uptime)}
        </div>
        <div className="font-mono text-[11px] text-muted-foreground">
          {formatLatency(window24h)}
        </div>
      </div>
    </Link>
  )
}

function CompactHeartbeats({ beats }: { beats: Heartbeat[] }) {
  const recent = beats.slice(-20)

  if (recent.length === 0) {
    return <div className="h-4 rounded bg-surface-elevated" />
  }

  return (
    <div
      className="grid h-4 gap-[2px] overflow-hidden rounded bg-surface-elevated p-[2px]"
      style={{ gridTemplateColumns: `repeat(${recent.length}, minmax(0, 1fr))` }}
    >
      {recent.map((beat, i) => {
        const status = normalizeStatus(beat.status)
        const tone = STATUS_TONES[status]
        return (
          <div
            key={`${beat.started_at}-${i}`}
            className={cn("min-w-0 rounded-[1px]", tone.bar)}
            title={`${statusLabels[status]} · ${formatTime(beat.started_at)} · ${beat.duration_ms} ms`}
          />
        )
      })}
    </div>
  )
}

function ServerMiniStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border/80 bg-background px-3 py-2">
      <div className="text-xs font-semibold uppercase tracking-normal text-muted-foreground">
        {label}
      </div>
      <div className="mt-1 font-mono text-sm font-semibold text-foreground">
        {value}
      </div>
    </div>
  )
}

function MonitorPanel({ monitor }: { monitor: StatusMonitor }) {
  const status = normalizeStatus(monitor.status)
  const tone = STATUS_TONES[status]
  const window24h = monitor.uptime?.windows?.["24h"]
  const Icon = monitorIcon(monitor)
  const cert = monitor.certificate

  return (
    <Link
      to={`/servers/${monitor.server_id}/monitors/${monitor.id}`}
      className={cn(
        "group block rounded-lg border bg-background p-3 transition hover:border-primary/70",
        status === "Healthy" ? "border-border/70" : tone.border,
        status !== "Healthy" && tone.softBg
      )}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 items-center gap-2">
            <Icon className={cn("size-4 shrink-0", tone.text)} />
            <span className="truncate text-sm font-medium text-foreground">
              {monitor.name}
            </span>
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span>{monitorKindLabels[monitor.kind] ?? monitor.kind}</span>
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
          <MonitorFact label="24h 可用性" value={uptimePct(monitor.uptime)} />
          <MonitorFact label="检查间隔" value={formatSeconds(monitor.interval_seconds)} />
          <MonitorFact label="最近检查" value={formatRelative(monitor.last_check_at)} />
        </div>
        {cert && <CertificateRing cert={cert} monitor={monitor} />}
      </div>

      <div className="mt-3">
        <div className="mb-1 text-xs font-semibold uppercase tracking-normal text-muted-foreground">
          最近心跳
        </div>
        <UptimeStrip beats={monitor.uptime?.heartbeats ?? []} />
      </div>
    </Link>
  )
}

function MonitorFact({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-xs font-semibold uppercase tracking-normal text-muted-foreground">
        {label}
      </div>
      <div className="mt-1 truncate font-mono text-sm text-foreground" title={value}>
        {value}
      </div>
    </div>
  )
}

function CertificateRing({
  cert,
  monitor,
}: {
  cert: StatusCertificate
  monitor: StatusMonitor
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
    <div className="flex items-center justify-end gap-3 rounded-lg border border-border bg-surface-elevated px-3 py-2">
      <div
        className="grid size-12 shrink-0 place-items-center rounded-full"
        style={{
          background: `conic-gradient(${tone.ring} ${progress}%, rgba(62,72,79,0.78) 0)`,
        }}
      >
        <div className="grid size-9 place-items-center rounded-full bg-background">
          <LockKeyhole className={cn("size-4", tone.text)} />
        </div>
      </div>
      <div className="min-w-0 text-right">
        <div className="text-xs font-semibold uppercase tracking-normal text-muted-foreground">
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
      <div className="h-6 rounded bg-surface-elevated px-2 text-xs leading-6 text-muted-foreground">
        暂无心跳数据
      </div>
    )
  }

  return (
    <div
      className="grid h-6 gap-[2px] overflow-hidden rounded bg-surface-elevated p-[2px]"
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

function monitorIcon(monitor: StatusMonitor) {
  if (monitor.kind === "url") return Globe2
  if (isTlsMonitor(monitor)) return ShieldCheck
  if (monitor.kind === "tcp") return Wifi
  return Terminal
}

function isTlsMonitor(monitor: StatusMonitor): boolean {
  return monitor.kind === "tls" || monitor.kind === "tls_port"
}

function filterGroups(groups: StatusGroup[], term: string): StatusGroup[] {
  const query = term.trim().toLowerCase()
  if (!query) return groups
  return groups
    .map((group) => {
      const groupMatch = [group.name, group.description]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(query))
      if (groupMatch) return group

      const servers = group.servers
        .map((server) => {
          const serverMatch = [server.name, server.environment, ...(server.tags ?? [])]
            .filter(Boolean)
            .some((value) => value!.toLowerCase().includes(query))
          if (serverMatch) return server
          const monitors = server.monitors.filter((monitor) =>
            monitorMatchesSearch(monitor, query)
          )
          return { ...server, monitors }
        })
        .filter((server) => server.monitors.length > 0)

      return { ...group, servers }
    })
    .filter((group) => group.servers.length > 0)
}

function monitorMatchesSearch(monitor: StatusMonitor, query: string): boolean {
  return [monitor.name, monitor.kind, monitorKindLabels[monitor.kind]]
    .filter((value): value is string => Boolean(value))
    .some((value) => value.toLowerCase().includes(query))
}
