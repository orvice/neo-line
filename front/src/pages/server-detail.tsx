import { useState } from "react"
import { Link, useParams } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ArrowLeft, Pencil, Play, Plus, Terminal, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { Monitor, Server, SshExecResponse, SshTestConnectionResponse } from "@/lib/types"
import { useAuth } from "@/lib/auth"
import {
  formatCertExpiry,
  formatRelative,
  formatTime,
  isTlsMonitorKind,
  monitorKindLabels,
} from "@/lib/format"
import { StatusBadge } from "@/components/status-badge"
import { MonitorForm } from "@/components/monitor-form"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { TableSkeleton } from "@/components/table-skeleton"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
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
          <TabsTrigger value="ssh">SSH 运维</TabsTrigger>
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
                          <div>{targetLabel(m)}</div>
                          {isTlsMonitorKind(m.kind) && m.certificate && (
                            <div className="mt-0.5">
                              证书 {formatCertExpiry(m.certificate)}
                            </div>
                          )}
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

        <TabsContent value="ssh">
          <SshPanel serverId={serverId} server={server} />
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

function SshPanel({ serverId, server }: { serverId: string; server: Server }) {
  const [command, setCommand] = useState("uptime")
  const [timeoutSeconds, setTimeoutSeconds] = useState(30)
  const [execResult, setExecResult] = useState<SshExecResponse | undefined>()
  const [testResult, setTestResult] = useState<SshTestConnectionResponse | undefined>()

  const ssh = server.ssh
  const enabled = Boolean(ssh?.enabled)
  const host = ssh?.host || server.host
  const port = ssh?.port || 22
  const sshUser = ssh?.user || "默认用户"

  const testMutation = useMutation({
    mutationFn: () => api.sshTestConnection(serverId),
    onSuccess: (res) => {
      setTestResult(res)
      toast.success(res.ok ? "SSH 连接正常" : "SSH 连接测试完成")
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "连接测试失败")
    },
  })

  const execMutation = useMutation({
    mutationFn: () =>
      api.sshExec(serverId, command.trim(), Math.max(1, timeoutSeconds || 30)),
    onSuccess: (res) => {
      setExecResult(res)
      toast.success(res.exit_code === 0 ? "命令执行完成" : `命令退出码 ${res.exit_code}`)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "命令执行失败")
    },
  })

  function submitExec() {
    if (!command.trim()) {
      toast.error("请输入要执行的命令")
      return
    }
    execMutation.mutate()
  }

  if (!enabled) {
    return (
      <Card>
        <CardContent className="text-muted-foreground flex flex-col items-center gap-2 p-10 text-center">
          <Terminal className="size-8 opacity-50" />
          此服务器未启用 SSH 执行，请在服务器编辑中启用。
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <CardTitle className="flex items-center gap-2 text-base">
                <Terminal className="size-4" />
                远程命令
              </CardTitle>
              <p className="text-muted-foreground mt-1 text-sm">
                {sshUser}@{host}:{port}
              </p>
            </div>
            <Button
              variant="outline"
              onClick={() => testMutation.mutate()}
              disabled={testMutation.isPending}
            >
              <Play />
              {testMutation.isPending ? "测试中…" : "测试连接"}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_140px]">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="ssh-command">命令</Label>
              <Input
                id="ssh-command"
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                onKeyDown={(e) => {
                  if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
                    submitExec()
                  }
                }}
                placeholder="uptime"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="ssh-timeout">超时（秒）</Label>
              <Input
                id="ssh-timeout"
                type="number"
                min={1}
                max={3600}
                value={timeoutSeconds}
                onChange={(e) => setTimeoutSeconds(Number(e.target.value))}
              />
            </div>
          </div>
          <div className="flex justify-end">
            <Button onClick={submitExec} disabled={execMutation.isPending}>
              <Terminal />
              {execMutation.isPending ? "执行中…" : "执行命令"}
            </Button>
          </div>
          {execResult && <ExecResult result={execResult} />}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">连接信息</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <InfoRow label="目标" value={host} mono />
          <InfoRow label="端口" value={String(port)} />
          <InfoRow label="用户" value={sshUser} />
          <InfoRow label="配置" value="已启用" />
          {testResult && (
            <div className="border-t pt-3">
              <div className="mb-2 flex items-center gap-2">
                <Badge variant={testResult.ok ? "secondary" : "destructive"}>
                  {testResult.ok ? "连接正常" : "连接失败"}
                </Badge>
                <span className="text-muted-foreground text-xs">
                  {testResult.host}
                </span>
              </div>
              {testResult.output && (
                <pre className="bg-muted/60 overflow-x-auto rounded-md border p-2 text-xs">
                  {testResult.output}
                </pre>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function ExecResult({ result }: { result: SshExecResponse }) {
  return (
    <div className="flex flex-col gap-3 border-t pt-4">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={result.exit_code === 0 ? "secondary" : "destructive"}>
          exit {result.exit_code}
        </Badge>
        <span className="text-muted-foreground text-xs">
          host {result.host}
        </span>
      </div>
      <OutputBlock label="stdout" value={result.stdout} />
      <OutputBlock label="stderr" value={result.stderr} destructive />
    </div>
  )
}

function OutputBlock({
  label,
  value,
  destructive,
}: {
  label: string
  value: string
  destructive?: boolean
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className={destructive ? "text-destructive text-xs" : "text-muted-foreground text-xs"}>
        {label}
      </span>
      <pre className="bg-muted/60 min-h-16 overflow-x-auto rounded-md border p-3 text-xs leading-relaxed">
        {value || "-"}
      </pre>
    </div>
  )
}

function InfoRow({
  label,
  value,
  mono,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className={mono ? "font-mono text-xs" : ""}>{value}</span>
    </div>
  )
}

function targetLabel(m: Monitor): string {
  if (m.kind === "url") return m.url ?? "-"
  if (m.host || m.port) return `${m.host ?? ""}${m.port ? `:${m.port}` : ""}`
  return "-"
}
