import { useState } from "react"
import { Link, useParams } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowLeft, Pencil, Plus, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { Monitor } from "@/lib/types"
import { useAuth } from "@/lib/auth"
import { formatRelative, formatTime, monitorKindLabels } from "@/lib/format"
import { StatusBadge } from "@/components/status-badge"
import { MonitorForm } from "@/components/monitor-form"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { TableSkeleton } from "@/components/table-skeleton"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

export function ServerDetailPage() {
  const { serverId = "" } = useParams()
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [monitorFormOpen, setMonitorFormOpen] = useState(false)
  const [editing, setEditing] = useState<Monitor | undefined>()
  const [deleting, setDeleting] = useState<Monitor | undefined>()

  const serverQuery = useQuery({
    queryKey: ["server", serverId],
    queryFn: () => api.getServer(serverId),
  })
  const healthQuery = useQuery({
    queryKey: ["server-health", serverId],
    queryFn: () => api.getServerHealth(serverId),
  })
  const monitorsQuery = useQuery({
    queryKey: ["monitors", serverId],
    queryFn: () => api.listMonitors(serverId, { page_size: 200 }),
  })
  const eventsQuery = useQuery({
    queryKey: ["events", serverId],
    queryFn: () => api.listServerEvents(serverId, { page_size: 50 }),
  })

  const deleteMutation = useMutation({
    mutationFn: (monitorId: string) => api.deleteMonitor(serverId, monitorId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["monitors", serverId] })
      queryClient.invalidateQueries({ queryKey: ["server-health", serverId] })
      toast.success("监控项已删除")
      setDeleting(undefined)
    },
    onError: (err) =>
      toast.error(err instanceof ApiError ? err.message : "删除失败"),
  })

  const server = serverQuery.data?.server
  const health = healthQuery.data?.health
  const monitors = monitorsQuery.data?.monitors ?? []
  const events = eventsQuery.data?.events ?? []

  if (serverQuery.isLoading) {
    return <div className="text-muted-foreground py-10 text-center">加载中…</div>
  }
  if (serverQuery.isError || !server) {
    return (
      <div className="text-destructive py-10 text-center">
        {serverQuery.error instanceof ApiError
          ? serverQuery.error.message
          : "服务器不存在"}
      </div>
    )
  }

  return (
    <div className="animate-enter flex flex-col gap-6">
      <Button asChild variant="ghost" size="sm" className="w-fit -ml-2">
        <Link to="/">
          <ArrowLeft />
          返回列表
        </Link>
      </Button>

      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-semibold">{server.name}</h1>
            <StatusBadge status={server.health_status} />
          </div>
          <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-sm">
            <span className="font-mono">{server.host}</span>
            {server.environment && <span>环境：{server.environment}</span>}
            {server.region && <span>区域：{server.region}</span>}
            <span>最近检查：{formatRelative(server.last_check_at)}</span>
          </div>
          {server.tags && server.tags.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {server.tags.map((t) => (
                <span
                  key={t}
                  className="bg-muted rounded px-2 py-0.5 text-xs"
                >
                  {t}
                </span>
              ))}
            </div>
          )}
        </div>
        {user && (
          <Button
            onClick={() => {
              setEditing(undefined)
              setMonitorFormOpen(true)
            }}
          >
            <Plus />
            新增监控项
          </Button>
        )}
      </div>

      {health && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-6">
          <SummaryCard label="监控总数" value={health.total_monitors} />
          <SummaryCard label="正常" value={health.healthy_monitors} />
          <SummaryCard label="警告" value={health.warning_monitors} />
          <SummaryCard label="严重" value={health.critical_monitors} />
          <SummaryCard label="宕机" value={health.down_monitors} />
          <SummaryCard label="未知" value={health.unknown_monitors} />
        </div>
      )}

      <Tabs defaultValue="monitors">
        <TabsList>
          <TabsTrigger value="monitors">监控项 ({monitors.length})</TabsTrigger>
          <TabsTrigger value="events">状态事件 ({events.length})</TabsTrigger>
        </TabsList>

        <TabsContent value="monitors">
          <Card className="py-0">
            <CardContent className="px-0">
              {monitorsQuery.isLoading ? (
                <TableSkeleton rows={4} columns={user ? 7 : 6} />
              ) : monitors.length === 0 ? (
                <div className="text-muted-foreground p-10 text-center">
                  暂无监控项
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>状态</TableHead>
                      <TableHead>名称</TableHead>
                      <TableHead>类型</TableHead>
                      <TableHead>目标</TableHead>
                      <TableHead>间隔</TableHead>
                      <TableHead>最近检查</TableHead>
                      {user && <TableHead className="text-right">操作</TableHead>}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {monitors.map((m) => (
                      <TableRow key={m.id}>
                        <TableCell>
                          <StatusBadge status={m.status} />
                        </TableCell>
                        <TableCell className="font-medium">
                          <Link
                            to={`/servers/${serverId}/monitors/${m.id}`}
                            className="hover:underline"
                          >
                            {m.name}
                          </Link>
                          {!m.enabled && (
                            <span className="text-muted-foreground ml-2 text-xs">
                              (已停用)
                            </span>
                          )}
                        </TableCell>
                        <TableCell className="text-sm">
                          {monitorKindLabels[m.kind] ?? m.kind}
                        </TableCell>
                        <TableCell className="text-muted-foreground font-mono text-xs">
                          {targetLabel(m)}
                        </TableCell>
                        <TableCell className="text-muted-foreground text-sm">
                          {m.interval_seconds}s
                        </TableCell>
                        <TableCell className="text-muted-foreground text-sm">
                          {formatRelative(m.last_check_at)}
                        </TableCell>
                        {user && (
                          <TableCell className="text-right">
                            <div className="flex justify-end gap-1">
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => {
                                  setEditing(m)
                                  setMonitorFormOpen(true)
                                }}
                              >
                                <Pencil />
                              </Button>
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => setDeleting(m)}
                              >
                                <Trash2 className="text-destructive" />
                              </Button>
                            </div>
                          </TableCell>
                        )}
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="events">
          <Card className="py-0">
            <CardContent className="px-0">
              {events.length === 0 ? (
                <div className="text-muted-foreground p-10 text-center">
                  暂无状态变更事件
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>时间</TableHead>
                      <TableHead>变化</TableHead>
                      <TableHead>原因</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {events.map((e) => (
                      <TableRow key={e.id}>
                        <TableCell className="text-sm">
                          {formatTime(e.occurred_at)}
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <StatusBadge status={e.previous_status} />
                            <span className="text-muted-foreground">→</span>
                            <StatusBadge status={e.current_status} />
                          </div>
                        </TableCell>
                        <TableCell className="text-muted-foreground text-sm">
                          {e.reason || "-"}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <MonitorForm
        open={monitorFormOpen}
        onOpenChange={setMonitorFormOpen}
        serverId={serverId}
        monitor={editing}
      />
      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(o) => !o && setDeleting(undefined)}
        title="删除监控项"
        description={`确定要删除「${deleting?.name}」吗？该操作不可恢复。`}
        pending={deleteMutation.isPending}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
    </div>
  )
}

function SummaryCard({ label, value }: { label: string; value: number }) {
  return (
    <Card>
      <CardHeader className="pb-0">
        <CardTitle className="text-muted-foreground text-xs font-normal">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <span className="text-2xl font-semibold">{value}</span>
      </CardContent>
    </Card>
  )
}

function targetLabel(m: Monitor): string {
  if (m.kind === "url") return m.url ?? "-"
  if (m.host || m.port) return `${m.host ?? ""}${m.port ? `:${m.port}` : ""}`
  return "-"
}
