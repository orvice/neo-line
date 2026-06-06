import { useMemo } from "react"
import { Link } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import {
  Activity,
  AlertTriangle,
  BellRing,
  FileSearch,
  FolderTree,
  LayoutDashboard,
  Plug,
  RefreshCw,
  Server as ServerIcon,
  ShieldCheck,
} from "lucide-react"

import { api, ApiError } from "@/lib/api"
import type {
  AuditLog,
  HealthStatus,
  StatusGroup,
  StatusMonitor,
  StatusServer,
} from "@/lib/types"
import {
  DEFAULT_TLS_WARNING_DAYS,
  formatCertExpiry,
  formatDuration,
  formatRelative,
  formatTime,
  isTlsMonitorKind,
  monitorKindLabels,
  statusLabels,
} from "@/lib/format"
import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { cn } from "@/lib/utils"

const statusRank: Record<HealthStatus, number> = {
  Healthy: 0,
  Unknown: 1,
  Warning: 2,
  Critical: 3,
  Down: 4,
}

const distributionOrder: HealthStatus[] = [
  "Down",
  "Critical",
  "Warning",
  "Unknown",
  "Healthy",
]

type MonitorRow = {
  group: StatusGroup
  server: StatusServer
  monitor: StatusMonitor
  status: HealthStatus
}

export function DashboardPage() {
  const overviewQuery = useQuery({
    queryKey: ["dashboard-status-overview"],
    queryFn: () => api.getStatusOverview(),
    refetchInterval: 60_000,
  })
  const serversQuery = useQuery({
    queryKey: ["dashboard-servers"],
    queryFn: () => api.listServers({ page_size: 200 }),
    refetchInterval: 60_000,
  })
  const groupsQuery = useQuery({
    queryKey: ["dashboard-monitor-groups"],
    queryFn: () => api.listMonitorGroups({ page_size: 200 }),
    refetchInterval: 60_000,
  })
  const notifyQuery = useQuery({
    queryKey: ["dashboard-notify-groups"],
    queryFn: () => api.listNotifyGroups({ page_size: 200 }),
    refetchInterval: 60_000,
  })
  const auditQuery = useQuery({
    queryKey: ["dashboard-audit-logs"],
    queryFn: () => api.listAuditLogs({ page_size: 8 }),
    refetchInterval: 60_000,
  })

  const groups = useMemo(
    () => overviewQuery.data?.groups ?? [],
    [overviewQuery.data]
  )
  const servers = serversQuery.data?.servers ?? []
  const monitorGroups = groupsQuery.data?.groups ?? []
  const notifyGroups = notifyQuery.data?.groups ?? []
  const auditLogs = auditQuery.data?.logs ?? []

  const monitorRows = useMemo(() => flattenMonitorRows(groups), [groups])
  const serverCounts = useMemo(
    () => countStatuses(servers.map((server) => server.health_status)),
    [servers]
  )
  const monitorCounts = useMemo(
    () => countStatuses(monitorRows.map((row) => row.status)),
    [monitorRows]
  )
  const problemMonitors = useMemo(
    () =>
      monitorRows
        .filter((row) => row.status !== "Healthy")
        .sort(compareMonitorRows)
        .slice(0, 8),
    [monitorRows]
  )
  const certificateRows = useMemo(
    () =>
      monitorRows
        .filter((row) => isCertificateConcern(row.monitor))
        .sort((a, b) => certDays(a.monitor) - certDays(b.monitor))
        .slice(0, 8),
    [monitorRows]
  )
  const globalUptime = useMemo(
    () => formatAverageUptime(monitorRows.map((row) => uptimeValue(row.monitor))),
    [monitorRows]
  )
  const lastUpdated = useMemo(
    () =>
      latestTime([
        ...servers.map((server) => server.last_check_at),
        ...monitorRows.map((row) => row.monitor.last_check_at),
      ]),
    [monitorRows, servers]
  )

  const enabledAlertPolicies = monitorGroups.filter(
    (group) => group.alert_policy?.enabled
  ).length
  const activeIssues =
    monitorCounts.Down + monitorCounts.Critical + monitorCounts.Warning
  const loading = overviewQuery.isLoading || serversQuery.isLoading
  const error = firstError([
    overviewQuery.error,
    serversQuery.error,
    groupsQuery.error,
    notifyQuery.error,
    auditQuery.error,
  ])

  function refreshAll() {
    overviewQuery.refetch()
    serversQuery.refetch()
    groupsQuery.refetch()
    notifyQuery.refetch()
    auditQuery.refetch()
  }

  return (
    <div className="animate-enter flex flex-col gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <LayoutDashboard className="text-brand size-5" />
            <h1 className="text-2xl font-semibold">运维总览</h1>
          </div>
          <p className="text-muted-foreground text-sm">
            最近刷新：{formatRelative(lastUpdated)}
          </p>
        </div>
        <Button
          variant="outline"
          size="icon"
          onClick={refreshAll}
          disabled={overviewQuery.isFetching || serversQuery.isFetching}
          title="刷新"
        >
          <RefreshCw
            className={
              overviewQuery.isFetching || serversQuery.isFetching
                ? "animate-spin"
                : ""
            }
          />
        </Button>
      </div>

      {error && (
        <Card className="border-destructive/30">
          <CardContent className="text-destructive p-4 text-sm">
            {error instanceof ApiError ? error.message : "部分数据加载失败"}
          </CardContent>
        </Card>
      )}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
        <SummaryTile
          icon={ServerIcon}
          label="服务器"
          value={loading ? "-" : String(servers.length)}
          note={`${serverCounts.Healthy} 正常 / ${serverProblemCount(serverCounts)} 异常`}
        />
        <SummaryTile
          icon={Activity}
          label="监控项"
          value={loading ? "-" : String(monitorRows.length)}
          note={`${monitorCounts.Healthy} 正常 / ${activeIssues} 需关注`}
        />
        <SummaryTile
          icon={AlertTriangle}
          label="活跃异常"
          value={loading ? "-" : String(activeIssues)}
          note={`${monitorCounts.Down} Down / ${monitorCounts.Critical} Critical`}
        />
        <SummaryTile
          icon={ShieldCheck}
          label="24h 可用率"
          value={globalUptime}
          note="按状态页监控项平均"
        />
        <SummaryTile
          icon={BellRing}
          label="告警策略"
          value={String(enabledAlertPolicies)}
          note={`${notifyGroups.length} 个通知组`}
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="flex flex-col gap-4">
          <Card className="py-0">
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <AlertTriangle className="size-4" />
                异常监控
              </CardTitle>
            </CardHeader>
            <CardContent className="px-0">
              <ProblemMonitorTable rows={problemMonitors} loading={loading} />
            </CardContent>
          </Card>

          <Card className="py-0">
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <ShieldCheck className="size-4" />
                TLS 证书关注
              </CardTitle>
            </CardHeader>
            <CardContent className="px-0">
              <CertificateTable rows={certificateRows} loading={loading} />
            </CardContent>
          </Card>
        </div>

        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">状态分布</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              {distributionOrder.map((status) => (
                <DistributionBar
                  key={status}
                  status={status}
                  count={monitorCounts[status]}
                  total={Math.max(1, monitorRows.length)}
                />
              ))}
            </CardContent>
          </Card>

          <Card className="py-0">
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <FileSearch className="size-4" />
                最近审计
              </CardTitle>
            </CardHeader>
            <CardContent className="px-0">
              <AuditList logs={auditLogs} loading={auditQuery.isLoading} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">快捷入口</CardTitle>
            </CardHeader>
            <CardContent className="grid grid-cols-2 gap-2">
              <QuickLink to="/servers" icon={ServerIcon} label="服务器" />
              <QuickLink to="/monitor-groups" icon={FolderTree} label="分组" />
              <QuickLink to="/notify-groups" icon={BellRing} label="通知组" />
              <QuickLink to="/audit-logs" icon={FileSearch} label="审计" />
              <QuickLink to="/mcp" icon={Plug} label="MCP" />
              <QuickLink to="/" icon={Activity} label="状态页" />
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}

function SummaryTile({
  icon: Icon,
  label,
  value,
  note,
}: {
  icon: typeof Activity
  label: string
  value: string
  note: string
}) {
  return (
    <Card>
      <CardContent className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-col gap-1">
          <span className="text-muted-foreground text-xs">{label}</span>
          <span className="text-2xl font-semibold tabular-nums">{value}</span>
          <span className="text-muted-foreground truncate text-xs">{note}</span>
        </div>
        <div className="bg-muted flex size-8 shrink-0 items-center justify-center rounded-md">
          <Icon className="size-4" />
        </div>
      </CardContent>
    </Card>
  )
}

function ProblemMonitorTable({
  rows,
  loading,
}: {
  rows: MonitorRow[]
  loading: boolean
}) {
  if (loading) {
    return <EmptyBlock text="加载中…" />
  }
  if (rows.length === 0) {
    return <EmptyBlock text="暂无异常监控" />
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>状态</TableHead>
          <TableHead>监控项</TableHead>
          <TableHead>位置</TableHead>
          <TableHead>24h</TableHead>
          <TableHead>最近检查</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow key={`${row.group.id}-${row.monitor.id}`}>
            <TableCell>
              <StatusBadge status={row.monitor.status} />
            </TableCell>
            <TableCell className="font-medium">
              <Link
                to={`/servers/${row.monitor.server_id}/monitors/${row.monitor.id}`}
                className="hover:underline"
              >
                {row.monitor.name}
              </Link>
              <div className="text-muted-foreground mt-0.5 text-xs">
                {monitorKindLabels[row.monitor.kind] ?? row.monitor.kind}
              </div>
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              <Link to={`/monitor-groups/${row.group.id}`} className="hover:underline">
                {row.group.name}
              </Link>
              <div className="text-xs">{row.server.name}</div>
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {formatMonitorUptime(row.monitor)}
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {formatRelative(row.monitor.last_check_at)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function CertificateTable({
  rows,
  loading,
}: {
  rows: MonitorRow[]
  loading: boolean
}) {
  if (loading) {
    return <EmptyBlock text="加载中…" />
  }
  if (rows.length === 0) {
    return <EmptyBlock text="暂无临期证书" />
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>监控项</TableHead>
          <TableHead>位置</TableHead>
          <TableHead>证书</TableHead>
          <TableHead>阈值</TableHead>
          <TableHead>状态</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow key={`${row.group.id}-${row.monitor.id}`}>
            <TableCell className="font-medium">
              <Link
                to={`/servers/${row.monitor.server_id}/monitors/${row.monitor.id}`}
                className="hover:underline"
              >
                {row.monitor.name}
              </Link>
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {row.group.name}
              <div className="text-xs">{row.server.name}</div>
            </TableCell>
            <TableCell className="text-sm">
              {formatCertExpiry(row.monitor.certificate)}
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              ≤ {row.monitor.warning_days || DEFAULT_TLS_WARNING_DAYS} 天
            </TableCell>
            <TableCell>
              <StatusBadge status={row.monitor.status} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function DistributionBar({
  status,
  count,
  total,
}: {
  status: HealthStatus
  count: number
  total: number
}) {
  const pct = Math.round((count / total) * 100)
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between gap-2 text-sm">
        <span>{statusLabels[status]}</span>
        <span className="text-muted-foreground tabular-nums">{count}</span>
      </div>
      <div className="bg-muted h-2 overflow-hidden rounded-full">
        <div
          className={cn("h-full rounded-full", statusBarClass(status))}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  )
}

function AuditList({
  logs,
  loading,
}: {
  logs: AuditLog[]
  loading: boolean
}) {
  if (loading) {
    return <EmptyBlock text="加载中…" />
  }
  if (logs.length === 0) {
    return <EmptyBlock text="暂无审计日志" />
  }
  return (
    <div className="flex flex-col divide-y">
      {logs.map((log) => (
        <Link
          key={log.id}
          to="/audit-logs"
          className="hover:bg-accent flex flex-col gap-1 px-4 py-3 transition"
        >
          <div className="flex items-center justify-between gap-2">
            <div className="flex min-w-0 items-center gap-1.5">
              <Badge variant="secondary">{log.source.toUpperCase()}</Badge>
              <span className="truncate text-sm font-medium">
                {log.action}
              </span>
            </div>
            <Badge variant={log.success ? "secondary" : "destructive"}>
              {log.success ? "成功" : "失败"}
            </Badge>
          </div>
          <div className="text-muted-foreground flex items-center justify-between gap-2 text-xs">
            <span className="truncate">
              {log.actor_email || log.token_prefix || log.resource_type || "-"}
            </span>
            <span className="shrink-0 tabular-nums">
              {formatDuration(log.duration_ms)}
            </span>
          </div>
          <span className="text-muted-foreground text-xs">
            {formatTime(log.occurred_at)}
          </span>
        </Link>
      ))}
    </div>
  )
}

function QuickLink({
  to,
  icon: Icon,
  label,
}: {
  to: string
  icon: typeof Activity
  label: string
}) {
  return (
    <Button asChild variant="outline" className="justify-start">
      <Link to={to}>
        <Icon className="size-4" />
        {label}
      </Link>
    </Button>
  )
}

function EmptyBlock({ text }: { text: string }) {
  return (
    <div className="text-muted-foreground p-8 text-center text-sm">{text}</div>
  )
}

function flattenMonitorRows(groups: StatusGroup[]): MonitorRow[] {
  return groups.flatMap((group) =>
    group.servers.flatMap((server) =>
      server.monitors.map((monitor) => ({
        group,
        server,
        monitor,
        status: normalizeStatus(monitor.status),
      }))
    )
  )
}

function countStatuses(statuses: string[]): Record<HealthStatus, number> {
  const counts: Record<HealthStatus, number> = {
    Healthy: 0,
    Warning: 0,
    Critical: 0,
    Down: 0,
    Unknown: 0,
  }
  for (const status of statuses) {
    counts[normalizeStatus(status)] += 1
  }
  return counts
}

function normalizeStatus(status: string): HealthStatus {
  return status in statusRank ? (status as HealthStatus) : "Unknown"
}

function compareMonitorRows(a: MonitorRow, b: MonitorRow): number {
  const rankDiff = statusRank[b.status] - statusRank[a.status]
  if (rankDiff !== 0) return rankDiff
  return timestamp(b.monitor.last_check_at) - timestamp(a.monitor.last_check_at)
}

function serverProblemCount(counts: Record<HealthStatus, number>): number {
  return counts.Down + counts.Critical + counts.Warning + counts.Unknown
}

function uptimeValue(monitor: StatusMonitor): number | undefined {
  const win = monitor.uptime?.windows?.["24h"]
  if (!win || win.total === 0) return undefined
  return win.uptime * 100
}

function formatMonitorUptime(monitor: StatusMonitor): string {
  const value = uptimeValue(monitor)
  return value === undefined ? "-" : `${value.toFixed(2)}%`
}

function formatAverageUptime(values: Array<number | undefined>): string {
  const valid = values.filter((value): value is number => value !== undefined)
  if (valid.length === 0) return "-"
  const avg = valid.reduce((sum, value) => sum + value, 0) / valid.length
  return `${avg.toFixed(2)}%`
}

function isCertificateConcern(monitor: StatusMonitor): boolean {
  if (!isTlsMonitorKind(monitor.kind) || !monitor.certificate) return false
  const days = monitor.certificate.days_remaining
  if (days === undefined) return false
  return days <= (monitor.warning_days || DEFAULT_TLS_WARNING_DAYS)
}

function certDays(monitor: StatusMonitor): number {
  return monitor.certificate?.days_remaining ?? Number.POSITIVE_INFINITY
}

function latestTime(values: Array<string | undefined>): string | undefined {
  const latest = values
    .map(timestamp)
    .filter((value) => value > 0)
    .sort((a, b) => a - b)
    .at(-1)
  return latest ? new Date(latest).toISOString() : undefined
}

function timestamp(value?: string): number {
  if (!value) return 0
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 0 : date.getTime()
}

function firstError(errors: unknown[]): Error | null {
  return errors.find((error): error is Error => error instanceof Error) ?? null
}

function statusBarClass(status: HealthStatus): string {
  switch (status) {
    case "Healthy":
      return "bg-emerald-500"
    case "Warning":
      return "bg-amber-500"
    case "Critical":
      return "bg-orange-500"
    case "Down":
      return "bg-red-500"
    default:
      return "bg-zinc-500"
  }
}
