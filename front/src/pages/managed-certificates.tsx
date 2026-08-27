import { useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { Plus, RefreshCw, Shield } from "lucide-react"

import { api, ApiError } from "@/lib/api"
import type { CertificateValidity, ManagedCertificate } from "@/lib/types"
import { useAuth } from "@/lib/auth"
import { CertificateNavTabs } from "@/components/certificate-nav-tabs"
import { ManagedCertificateForm } from "@/components/managed-certificate-form"
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

const VALIDITY_LABELS: Record<CertificateValidity, string> = {
  Missing: "Missing",
  Valid: "Valid",
  RenewalDue: "RenewalDue",
  Expired: "Expired",
  Revoked: "Revoked",
  Unspecified: "—",
}

function validityBadge(validity: CertificateValidity) {
  const base = "inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
  switch (validity) {
    case "Missing":
      return (
        <span className={`${base} bg-muted text-muted-foreground`}>Missing</span>
      )
    case "Valid":
      return (
        <span className={`${base} bg-emerald-500/15 text-emerald-700 dark:text-emerald-300`}>
          Valid
        </span>
      )
    case "RenewalDue":
      return (
        <span className={`${base} bg-amber-500/15 text-amber-700 dark:text-amber-300`}>
          RenewalDue
        </span>
      )
    case "Expired":
      return (
        <span className={`${base} bg-orange-500/15 text-orange-700 dark:text-orange-300`}>
          Expired
        </span>
      )
    case "Revoked":
      return (
        <span className={`${base} bg-red-500/15 text-red-700 dark:text-red-300`}>
          Revoked
        </span>
      )
    default:
      return (
        <span className={`${base} bg-muted text-muted-foreground`}>
          {VALIDITY_LABELS[validity]}
        </span>
      )
  }
}

function opStatusBadge(cert: ManagedCertificate) {
  const op = cert.latest_operation
  if (!op) return <span className="text-muted-foreground text-sm">—</span>
  const base = "inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
  if (op.status === "Pending") {
    return (
      <span className={`${base} bg-amber-500/15 text-amber-700 dark:text-amber-300`}>
        Issue · Pending
      </span>
    )
  }
  if (op.status === "Running") {
    return (
      <span className={`${base} bg-blue-500/15 text-blue-700 dark:text-blue-300`}>
        Issue · Running
      </span>
    )
  }
  return (
    <span className="text-muted-foreground text-sm">
      {op.type} · {op.status}
    </span>
  )
}

export function ManagedCertificatesPage() {
  const { user } = useAuth()
  const [search, setSearch] = useState("")
  const [formOpen, setFormOpen] = useState(false)

  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ["managed-certificates"],
    queryFn: () => api.listManagedCertificates({ page_size: 200 }),
    refetchInterval: (q) => {
      const certs = q.state.data?.certificates ?? []
      return certs.some(
        (c) =>
          c.latest_operation?.status === "Pending" ||
          c.latest_operation?.status === "Running"
      )
        ? 3000
        : false
    },
  })

  const certificates = data?.certificates ?? []

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return certificates
    return certificates.filter(
      (c) =>
        c.name.toLowerCase().includes(q) ||
        c.domains.some((d) => d.toLowerCase().includes(q))
    )
  }, [certificates, search])

  return (
    <div className="animate-enter flex flex-col gap-6">
      <CertificateNavTabs />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">托管证书</h1>
          <p className="text-muted-foreground text-sm">
            共 {certificates.length} 张 ManagedCertificate；创建后自动提交 Pending Issue
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
          {user ? (
            <Button onClick={() => setFormOpen(true)}>
              <Plus className="size-4" />
              新增证书
            </Button>
          ) : null}
        </div>
      </div>

      <Input
        placeholder="搜索名称或域名…"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-sm"
      />

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <TableSkeleton columns={6} rows={5} />
          ) : isError ? (
            <div className="text-destructive p-6 text-sm">
              {error instanceof ApiError ? error.message : "加载失败"}
            </div>
          ) : filtered.length === 0 ? (
            <div className="text-muted-foreground flex flex-col items-center gap-2 p-10 text-center text-sm">
              <Shield className="size-8 opacity-40" />
              <p>暂无托管证书</p>
              {user ? (
                <Button variant="outline" size="sm" onClick={() => setFormOpen(true)}>
                  创建第一张证书
                </Button>
              ) : null}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>名称</TableHead>
                  <TableHead>主域名</TableHead>
                  <TableHead>有效性</TableHead>
                  <TableHead>Operation</TableHead>
                  <TableHead>创建时间</TableHead>
                  <TableHead className="w-[80px]" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {filtered.map((c) => (
                  <TableRow key={c.id}>
                    <TableCell className="font-medium">{c.name}</TableCell>
                    <TableCell className="text-muted-foreground max-w-[200px] truncate">
                      {c.domains[0] ?? "—"}
                      {c.domains.length > 1 ? (
                        <span className="text-xs"> +{c.domains.length - 1}</span>
                      ) : null}
                    </TableCell>
                    <TableCell>{validityBadge(c.active_validity)}</TableCell>
                    <TableCell>{opStatusBadge(c)}</TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {formatTime(c.created_at)}
                    </TableCell>
                    <TableCell>
                      <Button asChild variant="ghost" size="sm">
                        <Link to={`/certificates/managed/${c.id}`}>详情</Link>
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <ManagedCertificateForm open={formOpen} onOpenChange={setFormOpen} />
    </div>
  )
}
