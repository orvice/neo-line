import { useMemo, useState } from "react"
import { CertificateNavTabs } from "@/components/certificate-nav-tabs"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { KeyRound, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { DNSProviderAccount } from "@/lib/types"
import { useAuth } from "@/lib/auth"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { DNSProviderAccountForm } from "@/components/dns-provider-account-form"
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
import { formatTime } from "@/lib/format"

export function DNSProviderAccountsPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState("")
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<DNSProviderAccount | undefined>()
  const [deleting, setDeleting] = useState<DNSProviderAccount | undefined>()

  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["dns-provider-accounts"],
    queryFn: () => api.listDNSProviderAccounts({ page_size: 200 }),
  })

  const accounts = data?.accounts ?? []

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return accounts
    return accounts.filter(
      (a) =>
        a.name.toLowerCase().includes(q) ||
        a.provider.toLowerCase().includes(q)
    )
  }, [accounts, search])

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteDNSProviderAccount(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["dns-provider-accounts"] })
      toast.success("DNS 账户已删除")
      setDeleting(undefined)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "删除失败")
    },
  })

  return (
    <div className="animate-enter flex flex-col gap-6">
      <CertificateNavTabs />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">DNS 账户</h1>
          <p className="text-muted-foreground text-sm">
            共 {accounts.length} 个 Cloudflare DNS 账户，用于 ACME DNS-01 验证
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
              新增 DNS 账户
            </Button>
          )}
        </div>
      </div>

      <Input
        placeholder="搜索名称或提供商…"
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
              <KeyRound className="size-8 opacity-50" />
              暂无 DNS 账户
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead>提供商</TableHead>
                  <TableHead>传播超时</TableHead>
                  <TableHead>Token</TableHead>
                  <TableHead>最近验证</TableHead>
                  {user && <TableHead className="text-right">操作</TableHead>}
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((a) => (
                  <TableRow key={a.id}>
                    <TableCell className="font-medium">{a.name}</TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {a.provider}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {a.propagation_timeout_seconds}s
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {a.token_configured ? "已配置" : "未配置"}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {a.token_last_verified_at
                        ? formatTime(a.token_last_verified_at)
                        : "-"}
                    </TableCell>
                    {user && (
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => {
                              setEditing(a)
                              setFormOpen(true)
                            }}
                            title="编辑或轮换 Token"
                          >
                            <Pencil />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => setDeleting(a)}
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

      <DNSProviderAccountForm
        open={formOpen}
        onOpenChange={setFormOpen}
        account={editing}
      />
      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(o) => !o && setDeleting(undefined)}
        title="删除 DNS 账户"
        description={`确定要删除「${deleting?.name}」吗？引用该账户的托管证书将无法继续 DNS-01 验证。`}
        pending={deleteMutation.isPending}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
    </div>
  )
}
