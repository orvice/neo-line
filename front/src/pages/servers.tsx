import { useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Pencil, Plus, RefreshCw, Server as ServerIcon, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { HealthStatus, Server } from "@/lib/types"
import { useAuth } from "@/lib/auth"
import { formatRelative, statusLabels } from "@/lib/format"
import { StatusBadge } from "@/components/status-badge"
import { ServerForm } from "@/components/server-form"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { TableSkeleton } from "@/components/table-skeleton"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

const summaryOrder: HealthStatus[] = [
  "Healthy",
  "Warning",
  "Critical",
  "Down",
  "Unknown",
]

export function ServersPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState("")
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Server | undefined>()
  const [deleting, setDeleting] = useState<Server | undefined>()

  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["servers"],
    queryFn: () => api.listServers({ page_size: 200 }),
  })

  const servers = data?.servers ?? []

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return servers
    return servers.filter(
      (s) =>
        s.name.toLowerCase().includes(q) ||
        s.host.toLowerCase().includes(q) ||
        (s.environment ?? "").toLowerCase().includes(q) ||
        (s.tags ?? []).some((t) => t.toLowerCase().includes(q))
    )
  }, [servers, search])

  const summary = useMemo(() => {
    const counts: Record<HealthStatus, number> = {
      Healthy: 0,
      Warning: 0,
      Critical: 0,
      Down: 0,
      Unknown: 0,
    }
    for (const s of servers) {
      const key = (s.health_status in counts ? s.health_status : "Unknown") as HealthStatus
      counts[key]++
    }
    return counts
  }, [servers])

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteServer(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["servers"] })
      toast.success("服务器已删除")
      setDeleting(undefined)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "删除失败")
    },
  })

  return (
    <div className="animate-enter flex flex-col gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">服务器</h1>
          <p className="text-muted-foreground text-sm">
            共 {servers.length} 台服务器
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="icon"
            onClick={() => refetch()}
            disabled={isFetching}
            title="刷新"
          >
            <RefreshCw className={isFetching ? "animate-spin" : ""} />
          </Button>
          {user && (
            <Button
              onClick={() => {
                setEditing(undefined)
                setFormOpen(true)
              }}
            >
              <Plus />
              新增服务器
            </Button>
          )}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">
        {summaryOrder.map((status) => (
          <Card key={status}>
            <CardContent className="flex flex-col gap-1">
              <span className="text-muted-foreground text-xs">
                {statusLabels[status]}
              </span>
              {isLoading ? (
                <Skeleton className="h-8 w-10" />
              ) : (
                <span className="text-2xl font-semibold tabular-nums">
                  {summary[status]}
                </span>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      <Input
        placeholder="搜索名称、主机、环境或标签…"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-sm"
      />

      <Card className="py-0">
        <CardContent className="px-0">
          {isLoading ? (
            <TableSkeleton rows={6} columns={user ? 7 : 6} />
          ) : isError ? (
            <div className="text-destructive p-8 text-center">
              {error instanceof ApiError ? error.message : "加载失败"}
            </div>
          ) : filtered.length === 0 ? (
            <div className="text-muted-foreground flex flex-col items-center gap-2 p-10 text-center">
              <ServerIcon className="size-8 opacity-50" />
              暂无服务器
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>状态</TableHead>
                  <TableHead>名称</TableHead>
                  <TableHead>主机</TableHead>
                  <TableHead>环境</TableHead>
                  <TableHead>排序</TableHead>
                  <TableHead>最近检查</TableHead>
                  {user && <TableHead className="text-right">操作</TableHead>}
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((s) => (
                  <TableRow key={s.id}>
                    <TableCell>
                      <StatusBadge status={s.health_status} />
                    </TableCell>
                    <TableCell className="font-medium">
                      <Link
                        to={`/servers/${s.id}`}
                        className="hover:underline"
                      >
                        {s.name}
                      </Link>
                      {!s.enabled && (
                        <span className="text-muted-foreground ml-2 text-xs">
                          (已停用)
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground font-mono text-xs">
                      {s.host}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {s.environment || "-"}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm tabular-nums">
                      {s.sort_order ?? 0}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {formatRelative(s.last_check_at)}
                    </TableCell>
                    {user && (
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => {
                              setEditing(s)
                              setFormOpen(true)
                            }}
                          >
                            <Pencil />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => setDeleting(s)}
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

      <ServerForm
        open={formOpen}
        onOpenChange={setFormOpen}
        server={editing}
      />
      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(o) => !o && setDeleting(undefined)}
        title="删除服务器"
        description={`确定要删除「${deleting?.name}」吗？该操作会同时删除其下所有监控项，且不可恢复。`}
        pending={deleteMutation.isPending}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
    </div>
  )
}
