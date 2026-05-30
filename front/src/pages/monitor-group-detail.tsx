import { Link, useParams } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { BellRing, ChevronLeft } from "lucide-react"

import { api, ApiError } from "@/lib/api"
import { formatRelative, monitorKindLabels, statusLabels } from "@/lib/format"
import { StatusBadge } from "@/components/status-badge"
import { TableSkeleton } from "@/components/table-skeleton"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

export function MonitorGroupDetailPage() {
  const { groupId } = useParams<{ groupId: string }>()
  const id = groupId ?? ""

  const groupQuery = useQuery({
    queryKey: ["monitor-group", id],
    queryFn: () => api.getMonitorGroup(id),
    enabled: Boolean(id),
  })
  const monitorsQuery = useQuery({
    queryKey: ["monitor-group-monitors", id],
    queryFn: () => api.listMonitorsByGroup(id, { page_size: 200 }),
    enabled: Boolean(id),
  })

  const group = groupQuery.data?.group
  const monitors = monitorsQuery.data?.monitors ?? []

  return (
    <div className="animate-enter flex flex-col gap-6">
      <div>
        <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2">
          <Link to="/monitor-groups">
            <ChevronLeft className="size-4" />
            返回分组列表
          </Link>
        </Button>
        {groupQuery.isLoading ? (
          <div className="text-muted-foreground">加载中…</div>
        ) : groupQuery.isError ? (
          <div className="text-destructive">
            {groupQuery.error instanceof ApiError
              ? groupQuery.error.message
              : "加载失败"}
          </div>
        ) : group ? (
          <>
            <h1 className="text-2xl font-semibold">{group.name}</h1>
            {group.description && (
              <p className="text-muted-foreground text-sm">{group.description}</p>
            )}
          </>
        ) : null}
      </div>

      {group && (
        <Card>
          <CardContent className="flex flex-col gap-2 text-sm">
            <div className="flex items-center gap-2 font-medium">
              <BellRing className="size-4" />
              告警策略
            </div>
            {group.alert_policy?.enabled ? (
              <ul className="text-muted-foreground flex flex-col gap-1">
                <li>
                  触发条件：
                  {[
                    group.alert_policy.on_down && "Down",
                    group.alert_policy.on_critical && "Critical",
                    group.alert_policy.on_warning && "Warning",
                    group.alert_policy.on_recover && "恢复",
                  ]
                    .filter(Boolean)
                    .join("、") || "（未选择任何条件）"}
                </li>
                <li>
                  节流：
                  {group.alert_policy.min_interval_seconds
                    ? `${group.alert_policy.min_interval_seconds} 秒`
                    : "不节流"}
                </li>
                <li>
                  通道：{group.alert_policy.channels?.length ?? 0} 个 webhook
                </li>
              </ul>
            ) : (
              <p className="text-muted-foreground">未启用</p>
            )}
          </CardContent>
        </Card>
      )}

      <div>
        <h2 className="text-lg font-semibold">分组下的监控项</h2>
        <p className="text-muted-foreground text-sm">
          共 {monitors.length} 个监控项
        </p>
      </div>

      <Card className="py-0">
        <CardContent className="px-0">
          {monitorsQuery.isLoading ? (
            <TableSkeleton rows={5} columns={4} />
          ) : monitorsQuery.isError ? (
            <div className="text-destructive p-8 text-center">
              {monitorsQuery.error instanceof ApiError
                ? monitorsQuery.error.message
                : "加载失败"}
            </div>
          ) : monitors.length === 0 ? (
            <div className="text-muted-foreground p-10 text-center">
              该分组下暂无监控项
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>状态</TableHead>
                  <TableHead>名称</TableHead>
                  <TableHead>类型</TableHead>
                  <TableHead>最近检查</TableHead>
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
                        to={`/servers/${m.server_id}/monitors/${m.id}`}
                        className="hover:underline"
                      >
                        {m.name}
                      </Link>
                      {!m.enabled && (
                        <span className="text-muted-foreground ml-2 text-xs">
                          ({statusLabels.Unknown})
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {monitorKindLabels[m.kind] ?? m.kind}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {formatRelative(m.last_check_at)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
