import { useMemo } from "react"
import { Link } from "react-router-dom"
import { useQueries, useQuery } from "@tanstack/react-query"
import { CheckCircle2, AlertTriangle, XCircle, RefreshCw } from "lucide-react"

import { api } from "@/lib/api"
import type { HealthStatus, Monitor, MonitorUptime, Server } from "@/lib/types"
import { useSettings } from "@/lib/settings"
import { formatRelative } from "@/lib/format"
import { StatusDot } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

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

const CHIP_STYLES: Record<HealthStatus, string> = {
  Healthy:
    "bg-emerald-500/10 text-emerald-700 border-emerald-600/30 dark:bg-emerald-500/15 dark:text-emerald-400 dark:border-emerald-500/30",
  Warning:
    "bg-amber-500/10 text-amber-700 border-amber-600/30 dark:bg-amber-500/15 dark:text-amber-400 dark:border-amber-500/30",
  Critical:
    "bg-orange-500/10 text-orange-700 border-orange-600/30 dark:bg-orange-500/15 dark:text-orange-400 dark:border-orange-500/30",
  Down: "bg-red-500/10 text-red-700 border-red-600/30 dark:bg-red-500/15 dark:text-red-400 dark:border-red-500/30",
  Unknown:
    "bg-zinc-500/10 text-zinc-600 border-zinc-500/30 dark:bg-zinc-500/15 dark:text-zinc-400",
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

  const loading = groupsQuery.isLoading
  const isFetching =
    groupsQuery.isFetching || monitorQueries.some((q) => q.isFetching)

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
                groupsQuery.refetch()
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
            暂无监控分组，请在分组中添加监控项后展示在状态页。
          </CardContent>
        </Card>
      )}

      {sections.map(({ group, monitors }) => {
        const serverRows = groupMonitorsByServer(monitors, serverMap)
        return (
          <Card key={group.id}>
            <CardContent className="flex flex-col gap-1 py-4">
              <div className="mb-1 flex items-baseline justify-between gap-2">
                <h2 className="font-semibold">{group.name}</h2>
                <Link
                  to={`/monitor-groups/${group.id}`}
                  className="text-muted-foreground hover:text-foreground text-xs"
                >
                  详情
                </Link>
              </div>
              {group.description && (
                <p className="text-muted-foreground mb-1 text-xs">
                  {group.description}
                </p>
              )}
              {serverRows.length === 0 ? (
                <p className="text-muted-foreground py-3 text-sm">
                  该分组下暂无启用的监控项
                </p>
              ) : (
                <div className="divide-y">
                  {serverRows.map((row) => (
                    <ServerRow
                      key={row.serverId}
                      row={row}
                      uptimeByMonitor={uptimeByMonitor}
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

function ServerRow({
  row,
  uptimeByMonitor,
}: {
  row: ServerRowData
  uptimeByMonitor: Map<string, MonitorUptime>
}) {
  const serverStatus = worst(row.monitors.map((m) => normalizeStatus(m.status)))
  const name = row.server?.name ?? row.serverId
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2 py-3">
      <Link
        to={`/servers/${row.serverId}`}
        className="flex shrink-0 items-center gap-2 text-sm font-medium hover:underline"
      >
        <StatusDot status={serverStatus} />
        <span className="truncate">{name}</span>
      </Link>
      <div className="flex flex-1 flex-wrap items-center gap-1.5">
        {row.monitors.map((m) => (
          <MonitorChip
            key={m.id}
            monitor={m}
            uptime={uptimeByMonitor.get(m.id)}
          />
        ))}
      </div>
    </div>
  )
}

function MonitorChip({
  monitor,
  uptime,
}: {
  monitor: Monitor
  uptime?: MonitorUptime
}) {
  const status = normalizeStatus(monitor.status)
  return (
    <Link
      to={`/servers/${monitor.server_id}/monitors/${monitor.id}`}
      title={`${monitor.kind} · ${monitor.status} · 24h ${uptimePct(uptime)}`}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium transition-opacity hover:opacity-80",
        CHIP_STYLES[status]
      )}
    >
      <StatusDot status={monitor.status} />
      <span className="truncate">{monitor.name}</span>
    </Link>
  )
}
