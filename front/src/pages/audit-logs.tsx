import { useState, type ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"
import { FileSearch, RefreshCw } from "lucide-react"

import { api, ApiError } from "@/lib/api"
import type { AuditLog, AuditLogQuery } from "@/lib/types"
import { formatDuration, formatTime } from "@/lib/format"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

type AuditFilters = {
  source: string
  action: string
  resource_type: string
  resource_id: string
  actor_email: string
  token_prefix: string
  success: string
  start_time: string
  end_time: string
}

const defaultFilters: AuditFilters = {
  source: "all",
  action: "",
  resource_type: "",
  resource_id: "",
  actor_email: "",
  token_prefix: "",
  success: "all",
  start_time: "",
  end_time: "",
}

const sourceLabels: Record<string, string> = {
  api: "API",
  mcp: "MCP",
}

const actionLabels: Record<string, string> = {
  read: "读取",
  create: "创建",
  update: "更新",
  delete: "删除",
}

const resourceTypeLabels: Record<string, string> = {
  server: "server",
  monitor: "monitor",
  monitor_group: "monitor group",
  notify_group: "notify group",
  mcp_token: "MCP token",
  ssh: "SSH",
  audit_log: "audit log",
  settings: "settings",
  auth: "auth",
}

export function AuditLogsPage() {
  const [filters, setFilters] = useState<AuditFilters>(defaultFilters)
  const [pageToken, setPageToken] = useState("")

  const query = buildQuery(filters, pageToken)
  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["audit-logs", query],
    queryFn: () => api.listAuditLogs(query),
  })

  const logs = data?.logs ?? []

  function updateFilter<K extends keyof AuditFilters>(
    key: K,
    value: AuditFilters[K]
  ) {
    setFilters((current) => ({ ...current, [key]: value }))
    setPageToken("")
  }

  function resetFilters() {
    setFilters(defaultFilters)
    setPageToken("")
  }

  return (
    <div className="animate-enter flex flex-col gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <FileSearch className="text-brand size-5" />
            <h1 className="text-2xl font-semibold">审计日志</h1>
          </div>
          <p className="text-muted-foreground text-sm">
            Connect API 与 MCP tool 调用记录
          </p>
        </div>
        <Button
          variant="outline"
          size="icon"
          onClick={() => refetch()}
          disabled={isFetching}
          title="刷新"
        >
          <RefreshCw className={isFetching ? "animate-spin" : ""} />
        </Button>
      </div>

      <Card>
        <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <FilterField label="来源">
            <Select
              value={filters.source}
              onValueChange={(value) => updateFilter("source", value)}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部来源</SelectItem>
                <SelectItem value="api">API</SelectItem>
                <SelectItem value="mcp">MCP</SelectItem>
              </SelectContent>
            </Select>
          </FilterField>
          <FilterField label="结果">
            <Select
              value={filters.success}
              onValueChange={(value) => updateFilter("success", value)}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部结果</SelectItem>
                <SelectItem value="success">成功</SelectItem>
                <SelectItem value="failed">失败</SelectItem>
              </SelectContent>
            </Select>
          </FilterField>
          <FilterField label="动作 / Tool">
            <Input
              value={filters.action}
              onChange={(e) => updateFilter("action", e.target.value)}
              placeholder="read / ssh_exec"
            />
          </FilterField>
          <FilterField label="资源类型">
            <Input
              value={filters.resource_type}
              onChange={(e) => updateFilter("resource_type", e.target.value)}
              placeholder="server / monitor"
            />
          </FilterField>
          <FilterField label="资源 ID">
            <Input
              value={filters.resource_id}
              onChange={(e) => updateFilter("resource_id", e.target.value)}
              placeholder="srv_..."
            />
          </FilterField>
          <FilterField label="用户邮箱">
            <Input
              value={filters.actor_email}
              onChange={(e) => updateFilter("actor_email", e.target.value)}
              placeholder="admin@example.com"
            />
          </FilterField>
          <FilterField label="Token 前缀">
            <Input
              value={filters.token_prefix}
              onChange={(e) => updateFilter("token_prefix", e.target.value)}
              placeholder="mcp_xxxx"
            />
          </FilterField>
          <div className="flex items-end gap-2">
            <Button variant="outline" onClick={resetFilters} className="w-full">
              重置筛选
            </Button>
          </div>
          <FilterField label="开始时间">
            <Input
              type="datetime-local"
              value={filters.start_time}
              onChange={(e) => updateFilter("start_time", e.target.value)}
            />
          </FilterField>
          <FilterField label="结束时间">
            <Input
              type="datetime-local"
              value={filters.end_time}
              onChange={(e) => updateFilter("end_time", e.target.value)}
            />
          </FilterField>
        </CardContent>
      </Card>

      <Card className="py-0">
        <CardContent className="px-0">
          {isLoading ? (
            <div className="text-muted-foreground p-8 text-center text-sm">
              加载中…
            </div>
          ) : isError ? (
            <div className="text-destructive p-8 text-center">
              {error instanceof ApiError ? error.message : "加载失败"}
            </div>
          ) : logs.length === 0 ? (
            <div className="text-muted-foreground flex flex-col items-center gap-2 p-10 text-center">
              <FileSearch className="size-8 opacity-50" />
              暂无审计日志
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>时间</TableHead>
                  <TableHead>来源 / 动作</TableHead>
                  <TableHead>调用方</TableHead>
                  <TableHead>资源</TableHead>
                  <TableHead>结果</TableHead>
                  <TableHead>目标 / 错误</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {logs.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell className="text-muted-foreground whitespace-nowrap text-sm">
                      {formatTime(log.occurred_at)}
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col gap-1">
                        <div className="flex flex-wrap items-center gap-1.5">
                          <Badge variant="secondary">
                            {sourceLabels[log.source] ?? log.source}
                          </Badge>
                          <code className="bg-muted rounded px-1.5 py-0.5 text-xs">
                            {actionLabels[log.action] ?? log.action}
                          </code>
                        </div>
                        {log.method && (
                          <span className="text-muted-foreground text-xs">
                            {log.method}
                            {log.status_code ? ` ${log.status_code}` : ""}
                          </span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <ActorCell log={log} />
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col gap-1">
                        <span className="text-sm">
                          {resourceTypeLabels[log.resource_type ?? ""] ??
                            log.resource_type ??
                            "-"}
                        </span>
                        {log.resource_id && (
                          <code className="text-muted-foreground text-xs">
                            {log.resource_id}
                          </code>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col gap-1">
                        <Badge
                          variant={log.success ? "secondary" : "destructive"}
                          className="w-fit"
                        >
                          {log.success ? "成功" : "失败"}
                        </Badge>
                        <span className="text-muted-foreground text-xs tabular-nums">
                          {formatDuration(log.duration_ms)}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="max-w-[360px]">
                      <div className="flex flex-col gap-1">
                        <span className="truncate font-mono text-xs">
                          {log.path || "-"}
                        </span>
                        {log.error && (
                          <span className="text-destructive line-clamp-2 text-xs">
                            {log.error}
                          </span>
                        )}
                        {(log.remote_ip || log.user_agent) && (
                          <span className="text-muted-foreground truncate text-xs">
                            {[log.remote_ip, log.user_agent].filter(Boolean).join(" · ")}
                          </span>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <div className="flex items-center justify-end gap-2">
        <Button
          variant="outline"
          disabled={!pageToken || isFetching}
          onClick={() => setPageToken("")}
        >
          返回第一页
        </Button>
        <Button
          variant="outline"
          disabled={!data?.next_page_token || isFetching}
          onClick={() => setPageToken(data?.next_page_token ?? "")}
        >
          下一页
        </Button>
      </div>
    </div>
  )
}

function FilterField({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label className="text-xs">{label}</Label>
      {children}
    </div>
  )
}

function ActorCell({ log }: { log: AuditLog }) {
  if (log.actor_email || log.actor_id) {
    return (
      <div className="flex flex-col gap-1">
        <span className="text-sm">{log.actor_email || log.actor_id}</span>
        {log.actor_id && log.actor_email && (
          <code className="text-muted-foreground text-xs">{log.actor_id}</code>
        )}
      </div>
    )
  }
  if (log.token_prefix) {
    return (
      <code className="bg-muted rounded px-1.5 py-0.5 text-xs">
        {log.token_prefix}…
      </code>
    )
  }
  return <span className="text-muted-foreground text-sm">-</span>
}

function buildQuery(filters: AuditFilters, pageToken: string): AuditLogQuery {
  return {
    page_size: 50,
    page_token: pageToken || undefined,
    source: filters.source === "all" ? undefined : filters.source,
    action: trimmed(filters.action),
    resource_type: trimmed(filters.resource_type),
    resource_id: trimmed(filters.resource_id),
    actor_email: trimmed(filters.actor_email),
    token_prefix: trimmed(filters.token_prefix),
    success:
      filters.success === "all" ? undefined : filters.success === "success",
    start_time: localDateTimeToISO(filters.start_time),
    end_time: localDateTimeToISO(filters.end_time),
  }
}

function trimmed(value: string): string | undefined {
  const next = value.trim()
  return next || undefined
}

function localDateTimeToISO(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString()
}
