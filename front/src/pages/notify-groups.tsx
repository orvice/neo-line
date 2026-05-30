import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { BellRing, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { NotifyGroup } from "@/lib/types"
import { useAuth } from "@/lib/auth"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { NotifyGroupForm } from "@/components/notify-group-form"
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

export function NotifyGroupsPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState("")
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<NotifyGroup | undefined>()
  const [deleting, setDeleting] = useState<NotifyGroup | undefined>()

  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["notify-groups"],
    queryFn: () => api.listNotifyGroups({ page_size: 200 }),
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
    mutationFn: (id: string) => api.deleteNotifyGroup(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["notify-groups"] })
      toast.success("通知组已删除")
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
          <h1 className="text-2xl font-semibold">通知组</h1>
          <p className="text-muted-foreground text-sm">
            共 {groups.length} 个通知组
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
              新增通知组
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
            <TableSkeleton rows={5} columns={user ? 4 : 3} />
          ) : isError ? (
            <div className="text-destructive p-8 text-center">
              {error instanceof ApiError ? error.message : "加载失败"}
            </div>
          ) : filtered.length === 0 ? (
            <div className="text-muted-foreground flex flex-col items-center gap-2 p-10 text-center">
              <BellRing className="size-8 opacity-50" />
              暂无通知组
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead>描述</TableHead>
                  <TableHead>通道</TableHead>
                  {user && <TableHead className="text-right">操作</TableHead>}
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((g) => (
                  <TableRow key={g.id}>
                    <TableCell className="font-medium">{g.name}</TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {g.description || "-"}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {g.channels?.length
                        ? `${g.channels.length} 个（${[
                            ...new Set(
                              g.channels.map((c) => c.type || "webhook")
                            ),
                          ].join("、")}）`
                        : "0"}
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

      <NotifyGroupForm
        open={formOpen}
        onOpenChange={setFormOpen}
        group={editing}
      />
      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(o) => !o && setDeleting(undefined)}
        title="删除通知组"
        description={`确定要删除「${deleting?.name}」吗？引用了该通知组的监控分组将不再向其通道派发告警。`}
        pending={deleteMutation.isPending}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
    </div>
  )
}
