import { useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { BellRing, FolderTree, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { MonitorGroup } from "@/lib/types"
import { useAuth } from "@/lib/auth"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { MonitorGroupForm } from "@/components/monitor-group-form"
import { TableSkeleton } from "@/components/table-skeleton"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

export function MonitorGroupsPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState("")
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<MonitorGroup | undefined>()
  const [deleting, setDeleting] = useState<MonitorGroup | undefined>()

  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["monitor-groups"],
    queryFn: () => api.listMonitorGroups({ page_size: 200 }),
  })

  const groups = data?.groups ?? []

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return groups
    return groups.filter(
      (g) =>
        g.name.toLowerCase().includes(q) ||
        (g.description ?? "").toLowerCase().includes(q)
    )
  }, [groups, search])

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteMonitorGroup(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["monitor-groups"] })
      toast.success("分组已删除")
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
          <h1 className="text-2xl font-semibold">监控分组</h1>
          <p className="text-muted-foreground text-sm">
            共 {groups.length} 个分组
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
              新增分组
            </Button>
          )}
        </div>
      </div>

      <Input
        placeholder="搜索名称或描述…"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-sm"
      />

      <Card className="py-0">
        <CardContent className="px-0">
          {isLoading ? (
            <TableSkeleton rows={5} columns={user ? 6 : 5} />
          ) : isError ? (
            <div className="text-destructive p-8 text-center">
              {error instanceof ApiError ? error.message : "加载失败"}
            </div>
          ) : filtered.length === 0 ? (
            <div className="text-muted-foreground flex flex-col items-center gap-2 p-10 text-center">
              <FolderTree className="size-8 opacity-50" />
              暂无分组
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead>描述</TableHead>
                  <TableHead>排序</TableHead>
                  <TableHead>告警</TableHead>
                  <TableHead>通知组</TableHead>
                  {user && <TableHead className="text-right">操作</TableHead>}
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((g) => (
                  <TableRow key={g.id}>
                    <TableCell className="font-medium">
                      <Link
                        to={`/monitor-groups/${g.id}`}
                        className="hover:underline"
                      >
                        {g.name}
                      </Link>
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {g.description || "-"}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm tabular-nums">
                      {g.sort_order ?? 0}
                    </TableCell>
                    <TableCell>
                      {g.alert_policy?.enabled ? (
                        <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
                          <BellRing className="size-3.5" />
                          已启用
                        </span>
                      ) : (
                        <span className="text-muted-foreground text-sm">
                          未启用
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {g.alert_policy?.notify_group_ids?.length ?? 0}
                    </TableCell>
                    {user && (
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => {
                              setEditing(g)
                              setFormOpen(true)
                            }}
                          >
                            <Pencil />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => setDeleting(g)}
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

      <MonitorGroupForm
        open={formOpen}
        onOpenChange={setFormOpen}
        group={editing}
      />
      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(o) => !o && setDeleting(undefined)}
        title="删除分组"
        description={`确定要删除「${deleting?.name}」吗？分组删除后，其下监控项仍保留，仅会从该分组中移除。`}
        pending={deleteMutation.isPending}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
    </div>
  )
}
