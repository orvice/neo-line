import { useMemo } from "react"
import { Link } from "react-router-dom"
import { useQueries, useQuery } from "@tanstack/react-query"
import { CheckCircle2, AlertTriangle, XCircle, RefreshCw } from "lucide-react"

import { api } from "@/lib/api"
import type { HealthStatus, Monitor, MonitorUptime } from "@/lib/types"
import { useSettings } from "@/lib/settings"
import { formatRelative } from "@/lib/format"
import { StatusDot } from "@/components/status-badge"
import { HeartbeatBar } from "@/components/heartbeat-bar"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"

const STATUS_RANK: Record<HealthStatus, number> = {
  Down: 4,
  Critical: 3,
  Warning: 2,
  Unknown: 1,
  Healthy: 0,
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

const OVERALL = {
  ok: {
    icon: CheckCircle2,
    title: "所有系统正常运行",
    className:
      "border-emerald-600/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
  },
  degraded: {
    icon: AlertTriangle,
    title: "部分系统出现异常",
    className:
      "border-amber-600/30 bg-amber-500/10 text-amber-700 dark:text-amber-400",
  },
  down: {
    icon: XCircle,
    title: "部分系统发生中断",
    className: "border-red-600/30 bg-red-500/10 text-red-700 dark:text-red-400",
  },
}

function uptimePct(uptime?: MonitorUptime): string {
  const win = uptime?.windows?.["24h"]
  if (!win || win.total === 0) return "-"
  return `${(win.uptime * 100).toFixed(2)}%`
}

export function StatusPage() {
  const settings = useSettings()
  const serversQuery = useQuery({
    queryKey: ["status-servers"],
    queryFn: () => api.listServers({ page_size: 200 }),
    refetchInterval: 60_000,
  })
  const servers = useMemo(
    () => (serversQuery.data?.servers ?? []).filter((s) => s.enabled),
    [serversQuery.data]
  )

  const monitorQueries = useQueries({
    queries: servers.map((s) => ({
      queryKey: ["status-server-monitors", s.id],
      queryFn: () => api.listMonitors(s.id, { page_size: 200 }),
      refetchInterval: 60_000,
    })),
  })

  const sections = useMemo(
    () =>
      servers.map((server, i) => ({
        server,
        monitors: (monitorQueries[i]?.data?.monitors ?? []).filter(
          (m) => m.enabled
        ),
      })),
    [servers, monitorQueries]
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

  const overallStatus = worst(
    allMonitors.map((m) => normalizeStatus(m.status))
  )
  const overall =
    overallStatus === "Down" || overallStatus === "Critical"
      ? OVERALL.down
      : overallStatus === "Warning"
        ? OVERALL.degraded
        : OVERALL.ok
  const OverallIcon = overall.icon

  const lastUpdated = useMemo(() => {
    const times = allMonitors
      .map((m) => m.last_check_at)
      .filter((t): t is string => Boolean(t))
      .sort()
    return times[times.length - 1]
  }, [allMonitors])

  const loading = serversQuery.isLoading
  const isFetching =
    serversQuery.isFetching || monitorQueries.some((q) => q.isFetching)

  return (
    <div className="animate-enter mx-auto flex max-w-3xl flex-col gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">
        {settings.status_page_title}
      </h1>

      {loading ? (
        <Skeleton className="h-20 w-full" />
      ) : (
        <Card className={overall.className + " border"}>
          <CardContent className="flex items-center gap-3 py-5">
            <OverallIcon className="size-7 shrink-0" />
            <div className="flex-1">
              <div className="text-lg font-semibold">{overall.title}</div>
              <div className="text-xs opacity-80">
                最近更新：{formatRelative(lastUpdated)}
              </div>
            </div>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => {
                serversQuery.refetch()
                monitorQueries.forEach((q) => q.refetch())
                uptimeQueries.forEach((q) => q.refetch())
              }}
              title="刷新"
            >
              <RefreshCw
                className={isFetching ? "animate-spin" : undefined}
              />
            </Button>
          </CardContent>
        </Card>
      )}

      {!loading && sections.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-10 text-center">
            暂无启用的服务器，请添加服务器并配置监控项后展示在状态页。
          </CardContent>
        </Card>
      )}

      {sections.map(({ server, monitors }) => {
        const serverStatus = monitors.length
          ? worst(monitors.map((m) => normalizeStatus(m.status)))
          : normalizeStatus(server.health_status)
        return (
          <Card key={server.id}>
            <CardContent className="flex flex-col gap-1 py-4">
              <div className="mb-1 flex items-baseline justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                  <StatusDot status={serverStatus} />
                  <h2 className="truncate font-semibold">{server.name}</h2>
                  <span className="text-muted-foreground truncate text-xs">
                    {server.host}
                  </span>
                </div>
                <Link
                  to={`/servers/${server.id}`}
                  className="text-muted-foreground hover:text-foreground shrink-0 text-xs"
                >
                  详情
                </Link>
              </div>
              {monitors.length === 0 ? (
                <p className="text-muted-foreground py-3 text-sm">
                  该服务器下暂无启用的监控项
                </p>
              ) : (
                <div className="divide-y">
                  {monitors.map((m) => (
                    <MonitorRow
                      key={m.id}
                      monitor={m}
                      uptime={uptimeByMonitor.get(m.id)}
                    />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}

function MonitorRow({
  monitor,
  uptime,
}: {
  monitor: Monitor
  uptime?: MonitorUptime
}) {
  return (
    <div className="flex items-center gap-3 py-2.5">
      <StatusDot status={monitor.status} />
      <Link
        to={`/servers/${monitor.server_id}/monitors/${monitor.id}`}
        className="flex min-w-0 flex-1 items-center gap-2 truncate text-sm font-medium hover:underline"
      >
        <span className="truncate">{monitor.name}</span>
        <span className="bg-muted text-muted-foreground shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide">
          {monitor.kind}
        </span>
      </Link>
      <div className="hidden sm:block">
        <HeartbeatBar beats={uptime?.heartbeats ?? []} max={30} />
      </div>
      <span className="text-muted-foreground w-16 text-right text-xs tabular-nums">
        {uptimePct(uptime)}
      </span>
    </div>
  )
}
