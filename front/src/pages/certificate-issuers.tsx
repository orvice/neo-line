import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { AlertTriangle, Pencil, Plus, RefreshCw, RotateCcw, Shield, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { CertificateIssuer } from "@/lib/types"
import { useAuth } from "@/lib/auth"
import { CertificateNavTabs } from "@/components/certificate-nav-tabs"
import { CertificateIssuerForm } from "@/components/certificate-issuer-form"
import { ConfirmDialog } from "@/components/confirm-dialog"
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

const CA_LABELS: Record<string, string> = {
  lets_encrypt_production: "Let's Encrypt 生产",
  lets_encrypt_staging: "Let's Encrypt Staging",
  zerossl: "ZeroSSL",
  google_public_ca: "Google Public CA",
  custom: "自定义",
}

function statusBadge(issuer: CertificateIssuer) {
  const base = "inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
  switch (issuer.registration_status) {
    case "Ready":
      return <span className={`${base} bg-emerald-500/15 text-emerald-700 dark:text-emerald-300`}>Ready</span>
    case "Pending":
      return <span className={`${base} bg-amber-500/15 text-amber-700 dark:text-amber-300`}>Pending</span>
    case "Failed":
      return <span className={`${base} bg-red-500/15 text-red-700 dark:text-red-300`}>Failed</span>
    default:
      return <span className={`${base} bg-muted text-muted-foreground`}>Unknown</span>
  }
}

export function CertificateIssuersPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState("")
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<CertificateIssuer | undefined>()
  const [deleting, setDeleting] = useState<CertificateIssuer | undefined>()

  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["certificate-issuers"],
    queryFn: () => api.listCertificateIssuers({ page_size: 200 }),
    refetchInterval: (q) => {
      const issuers = q.state.data?.issuers ?? []
      return issuers.some((i) => i.registration_status === "Pending") ? 3000 : false
    },
  })

  const issuers = data?.issuers ?? []

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return issuers
    return issuers.filter(
      (i) =>
        i.name.toLowerCase().includes(q) ||
        i.email.toLowerCase().includes(q) ||
        i.ca_type.toLowerCase().includes(q)
    )
  }, [issuers, search])

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteCertificateIssuer(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["certificate-issuers"] })
      toast.success("Issuer 已删除")
      setDeleting(undefined)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "删除失败")
    },
  })

  const retryMutation = useMutation({
    mutationFn: (id: string) => api.retryCertificateIssuerRegistration(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["certificate-issuers"] })
      toast.success("已重新发起注册")
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "重试失败")
    },
  })

  return (
    <div className="animate-enter flex flex-col gap-6">
      <CertificateNavTabs />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">ACME Issuer</h1>
          <p className="text-muted-foreground text-sm">
            共 {issuers.length} 个 ACME 账户配置；仅 Ready 状态可用于证书签发
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
              新增 Issuer
            </Button>
          )}
        </div>
      </div>

      <Input
        placeholder="搜索名称、邮箱或 CA…"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-sm"
      />

      <Card className="py-0">
        <CardContent className="px-0">
          {isLoading ? (
            <TableSkeleton rows={5} columns={user ? 8 : 7} />
          ) : isError ? (
            <div className="text-destructive p-8 text-center">
              {error instanceof ApiError ? error.message : "加载失败"}
            </div>
          ) : filtered.length === 0 ? (
            <div className="text-muted-foreground flex flex-col items-center gap-2 p-10 text-center">
              <Shield className="size-8 opacity-50" />
              暂无 ACME Issuer
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead>CA</TableHead>
                  <TableHead>邮箱</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>信任</TableHead>
                  <TableHead>Directory</TableHead>
                  <TableHead>更新时间</TableHead>
                  {user && <TableHead className="text-right">操作</TableHead>}
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((i) => (
                  <TableRow key={i.id}>
                    <TableCell className="font-medium">{i.name}</TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {CA_LABELS[i.ca_type] ?? i.ca_type}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">{i.email}</TableCell>
                    <TableCell>
                      <div className="flex flex-col gap-1">
                        {statusBadge(i)}
                        {i.registration_error ? (
                          <span className="text-destructive max-w-xs truncate text-xs" title={i.registration_error}>
                            {i.registration_error}
                          </span>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell>
                      {i.staging_untrusted ? (
                        <span className="inline-flex items-center gap-1 text-amber-600 text-xs dark:text-amber-400">
                          <AlertTriangle className="size-3.5" />
                          Staging / 不受信任
                        </span>
                      ) : (
                        <span className="text-muted-foreground text-xs">公共 CA</span>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground max-w-[12rem] truncate text-xs" title={i.directory_url}>
                      {i.directory_url}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {formatTime(i.updated_at)}
                    </TableCell>
                    {user && (
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-1">
                          {i.registration_status === "Failed" && (
                            <Button
                              variant="ghost"
                              size="icon"
                              title="重试注册"
                              disabled={retryMutation.isPending}
                              onClick={() => retryMutation.mutate(i.id)}
                            >
                              <RotateCcw />
                            </Button>
                          )}
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => {
                              setEditing(i)
                              setFormOpen(true)
                            }}
                            title="编辑"
                          >
                            <Pencil />
                          </Button>
                          <Button variant="ghost" size="icon" onClick={() => setDeleting(i)}>
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

      <CertificateIssuerForm open={formOpen} onOpenChange={setFormOpen} issuer={editing} />
      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(o) => !o && setDeleting(undefined)}
        title="删除 Issuer"
        description={`确定要删除「${deleting?.name}」吗？此操作仅删除本地配置，不会停用远端 ACME 账户。`}
        pending={deleteMutation.isPending}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
    </div>
  )
}
