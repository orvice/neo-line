import { Link, useParams } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { ArrowLeft } from "lucide-react"

import { api, ApiError } from "@/lib/api"
import type { CheckResult, UptimeWindow } from "@/lib/types"
import {
  DEFAULT_TLS_CRITICAL_DAYS,
  DEFAULT_TLS_WARNING_DAYS,
  formatDuration,
  formatRelative,
  formatTime,
  isTlsMonitorKind,
  monitorKindLabels,
} from "@/lib/format"
import { StatusBadge } from "@/components/status-badge"
import { HeartbeatBar } from "@/components/heartbeat-bar"
import { TableSkeleton } from "@/components/table-skeleton"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
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

export function MonitorDetailPage() {
  const { serverId = "", monitorId = "" } = useParams()

  const monitorQuery = useQuery({
    queryKey: ["monitor", serverId, monitorId],
    queryFn: () => api.getMonitor(serverId, monitorId),
  })
  const resultsQuery = useQuery({
    queryKey: ["results", serverId, monitorId],
    queryFn: () => api.listCheckResults(serverId, monitorId, { page_size: 100 }),
    refetchInterval: 30_000,
  })
  const uptimeQuery = useQuery({
    queryKey: ["uptime", serverId, monitorId],
    queryFn: () => api.getMonitorUptime(serverId, monitorId),
    refetchInterval: 30_000,
  })

  const monitor = monitorQuery.data?.monitor
  const results = resultsQuery.data?.results ?? []
  const uptime = uptimeQuery.data?.uptime

  if (monitorQuery.isLoading) {
    return <div className="text-muted-foreground py-10 text-center">加载中…</div>
  }
  if (monitorQuery.isError || !monitor) {
    return (
      <div className="text-destructive py-10 text-center">
        {monitorQuery.error instanceof ApiError
          ? monitorQuery.error.message
          : "监控项不存在"}
      </div>
    )
  }

  const latestCert =
    monitor.certificate ?? results.find((r) => r.certificate)?.certificate

  return (
    <div className="animate-enter flex flex-col gap-6">
      <Button asChild variant="ghost" size="sm" className="w-fit -ml-2">
        <Link to={`/servers/${serverId}`}>
          <ArrowLeft />
          返回服务器
        </Link>
      </Button>

      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-semibold">{monitor.name}</h1>
        <StatusBadge status={monitor.status} />
        <Badge variant="secondary">
          {monitorKindLabels[monitor.kind] ?? monitor.kind}
        </Badge>
        {!monitor.enabled && <Badge variant="outline">已停用</Badge>}
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle>可用率</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-wrap gap-x-10 gap-y-3">
            <UptimeStat label="最近 1 小时" window={uptime?.windows?.["1h"]} />
            <UptimeStat label="最近 24 小时" window={uptime?.windows?.["24h"]} />
          </div>
          <div>
            <div className="text-muted-foreground mb-1.5 text-xs">
              最近心跳
            </div>
            <HeartbeatBar beats={uptime?.heartbeats ?? []} />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>配置</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-x-8 gap-y-3 text-sm sm:grid-cols-3">
            <Field label="探测间隔" value={`${monitor.interval_seconds} 秒`} />
            <Field label="超时" value={`${monitor.timeout_seconds} 秒`} />
            <Field label="重试次数" value={String(monitor.retries)} />
            {(monitor.kind === "tcp" || isTlsMonitorKind(monitor.kind)) && (
              <Field
                label="目标"
                value={`${monitor.host || "(服务器主机)"}${monitor.port ? `:${monitor.port}` : ""}`}
              />
            )}
            {monitor.kind === "url" && (
              <>
                <Field label="URL" value={monitor.url ?? "-"} />
                <Field label="方法" value={monitor.method ?? "GET"} />
                <Field
                  label="期望状态码"
                  value={monitor.expected_status_codes || "-"}
                />
              </>
            )}
            {(monitor.kind === "url" || isTlsMonitorKind(monitor.kind)) && (
              <>
                <Field
                  label="TLS 校验"
                  value={monitor.tls_verify ? "开启" : "关闭"}
                />
                <Field label="SNI 名称" value={monitor.sni_name || "默认"} />
              </>
            )}
            {isTlsMonitorKind(monitor.kind) && (
              <>
                <Field
                  label="警告阈值"
                  value={`${monitor.warning_days ?? DEFAULT_TLS_WARNING_DAYS} 天`}
                />
                <Field
                  label="严重阈值"
                  value={`${monitor.critical_days ?? DEFAULT_TLS_CRITICAL_DAYS} 天`}
                />
              </>
            )}
            <Field
              label="最近检查"
              value={formatRelative(monitor.last_check_at)}
            />
          </dl>
        </CardContent>
      </Card>

      {latestCert && (
        <Card>
          <CardHeader>
            <CardTitle>证书信息</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-1 gap-x-8 gap-y-3 text-sm sm:grid-cols-2">
              <Field label="主题" value={latestCert.subject || "-"} />
              <Field label="颁发者" value={latestCert.issuer || "-"} />
              <Field
                label="有效期至"
                value={formatTime(latestCert.not_after)}
              />
              <Field
                label="剩余天数"
                value={
                  latestCert.days_remaining !== undefined
                    ? `${latestCert.days_remaining} 天`
                    : "-"
                }
              />
              <Field
                label="DNS 名称"
                value={latestCert.dns_names?.join(", ") || "-"}
              />
              <Field
                label="序列号"
                value={latestCert.serial_number || "-"}
              />
            </dl>
          </CardContent>
        </Card>
      )}

      <div>
        <h2 className="mb-3 text-lg font-semibold">检查历史</h2>
        <Card className="py-0">
          <CardContent className="px-0">
            {resultsQuery.isLoading ? (
              <TableSkeleton rows={6} columns={5} />
            ) : results.length === 0 ? (
              <div className="text-muted-foreground p-10 text-center">
                暂无检查记录
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>状态</TableHead>
                    <TableHead>时间</TableHead>
                    <TableHead>耗时</TableHead>
                    <TableHead>HTTP</TableHead>
                    <TableHead>详情</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {results.map((r) => (
                    <ResultRow key={r.id} result={r} />
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function ResultRow({ result }: { result: CheckResult }) {
  return (
    <TableRow>
      <TableCell>
        <StatusBadge status={result.status} />
      </TableCell>
      <TableCell className="text-sm">{formatTime(result.started_at)}</TableCell>
      <TableCell className="text-muted-foreground text-sm">
        {formatDuration(result.duration_ms)}
      </TableCell>
      <TableCell className="text-muted-foreground text-sm">
        {result.http_status_code || "-"}
      </TableCell>
      <TableCell className="text-muted-foreground max-w-md truncate text-xs">
        {result.error_message
          ? `${result.error_stage ? `[${result.error_stage}] ` : ""}${result.error_message}`
          : result.remote_address || "正常"}
      </TableCell>
    </TableRow>
  )
}

function UptimeStat({
  label,
  window,
}: {
  label: string
  window?: UptimeWindow
}) {
  const hasData = window && window.total > 0
  const pct = hasData ? (window.uptime * 100).toFixed(2) : "-"
  const color = !hasData
    ? "text-muted-foreground"
    : window.uptime >= 0.99
      ? "text-emerald-600 dark:text-emerald-400"
      : window.uptime >= 0.95
        ? "text-amber-600 dark:text-amber-400"
        : "text-red-600 dark:text-red-400"
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-muted-foreground text-xs">{label}</span>
      <span className={`text-2xl font-semibold ${color}`}>
        {pct}
        {hasData && <span className="text-base font-normal">%</span>}
      </span>
      {hasData && (
        <span className="text-muted-foreground text-xs">
          {window.total} 次检查，{window.down} 次失败
          {window.avg_latency_ms > 0 &&
            `，平均 ${Math.round(window.avg_latency_ms)} ms`}
        </span>
      )}
    </div>
  )
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="font-medium break-all">{value}</dd>
    </div>
  )
}
