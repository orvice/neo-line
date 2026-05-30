import { useMemo } from "react"
import { Link } from "react-router-dom"
import { useQueries, useQuery } from "@tanstack/react-query"
import { CheckCircle2, AlertTriangle, XCircle, RefreshCw } from "lucide-react"

import { api } from "@/lib/api"
import type { HealthStatus, Monitor, MonitorUptime } from "@/lib/types"
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
  const groupsQuery = useQuery({
    queryKey: ["status-groups"],
    queryFn: () => api.listMonitorGroups({ page_size: 200 }),
    refetchInterval: 60_000,
  })
  const groups = groupsQuery.data?.groups ?? []

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

      {sections.map(({ group, monitors }) => (
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
            {monitors.length === 0 ? (
              <p className="text-muted-foreground py-3 text-sm">
                该分组下暂无启用的监控项
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
      ))}
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
        className="min-w-0 flex-1 truncate text-sm font-medium hover:underline"
      >
        {monitor.name}
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
