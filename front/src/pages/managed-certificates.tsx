import { useMemo, useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  Loader2,
  Plus,
  RefreshCw,
  Repeat,
  Rocket,
  RotateCcw,
  Shield,
  X,
} from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { ManagedCertificate } from "@/lib/types"
import { useAuth } from "@/lib/auth"
import {
  activeDomains,
  certQueryKeys,
  managedCertListFilterLabel,
  matchesManagedCertFilter,
  operationInFlight,
  parseManagedCertListFilter,
} from "@/lib/certificate-ui"
import { formatManagedCertExpiry } from "@/lib/format"
import { CertificateNavTabs } from "@/components/certificate-nav-tabs"
import {
  CertificateAvailabilityBadge,
  CertificateOperationBadge,
  CertificateStagingBadge,
  CertificateValidityBadge,
} from "@/components/certificate-status-badges"
import { ManagedCertificateForm } from "@/components/managed-certificate-form"
import { TableSkeleton } from "@/components/table-skeleton"
import { Badge } from "@/components/ui/badge"
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

function AutoRenewCell({ enabled }: { enabled: boolean }) {
  return enabled ? (
    <span className="text-sm">开启</span>
  ) : (
    <span className="text-muted-foreground text-sm">关闭</span>
  )
}

function DomainsCell({ domains }: { domains: string[] }) {
  if (domains.length === 0) return <span className="text-muted-foreground">—</span>
  const primary = domains[0]
  return (
    <div className="min-w-[120px] max-w-[220px]">
      <div className="truncate font-mono text-xs" title={primary}>
        {primary}
      </div>
      {domains.length > 1 ? (
        <div className="text-muted-foreground truncate text-xs" title={domains.slice(1).join(", ")}>
          +{domains.length - 1} SAN
        </div>
      ) : null}
    </div>
  )
}

export function ManagedCertificatesPage() {
  const { user } = useAuth()
  const [searchParams, setSearchParams] = useSearchParams()
  const [search, setSearch] = useState("")
  const [formOpen, setFormOpen] = useState(false)
  const listFilter = parseManagedCertListFilter(searchParams)

  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: certQueryKeys.list,
    queryFn: () => api.listManagedCertificates({ page_size: 200 }),
    refetchInterval: (q) => {
      const certs = q.state.data?.certificates ?? []
      return certs.some((c) => operationInFlight(c.latest_operation)) ? 3000 : false
    },
  })

  const issuerQuery = useQuery({
    queryKey: certQueryKeys.issuers,
    queryFn: () => api.listCertificateIssuers({ page_size: 200 }),
  })

  const certificates = data?.certificates ?? []
  const issuerNames = useMemo(() => {
    const map = new Map<string, string>()
    for (const issuer of issuerQuery.data?.issuers ?? []) {
      map.set(issuer.id, issuer.name)
    }
    return map
  }, [issuerQuery.data])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return certificates.filter((c) => {
      if (!matchesManagedCertFilter(c, listFilter)) return false
      if (!q) return true
      return (
        c.name.toLowerCase().includes(q) ||
        c.domains.some((d) => d.toLowerCase().includes(q)) ||
        activeDomains(c).some((d) => d.toLowerCase().includes(q))
      )
    })
  }, [certificates, listFilter, search])

  function clearFilter() {
    setSearchParams({})
  }

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

      {listFilter.kind !== "all" ? (
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="secondary" className="gap-1 pr-1">
            筛选：{managedCertListFilterLabel(listFilter)}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-5"
              onClick={clearFilter}
              title="清除筛选"
            >
              <X className="size-3" />
            </Button>
          </Badge>
          <span className="text-muted-foreground text-sm">匹配 {filtered.length} 张</span>
        </div>
      ) : null}

      <Input
        placeholder="搜索名称或域名…"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-sm"
      />

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <TableSkeleton columns={10} rows={5} />
          ) : isError ? (
            <div className="text-destructive p-6 text-sm">
              {error instanceof ApiError ? error.message : "加载托管证书失败，请稍后重试"}
            </div>
          ) : filtered.length === 0 ? (
            <div className="text-muted-foreground flex flex-col items-center gap-2 p-10 text-center text-sm">
              <Shield className="size-8 opacity-40" />
              <p>{listFilter.kind === "all" ? "暂无托管证书" : "当前筛选下暂无证书"}</p>
              {user && listFilter.kind === "all" ? (
                <Button variant="outline" size="sm" onClick={() => setFormOpen(true)}>
                  创建第一张证书
                </Button>
              ) : null}
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="min-w-[120px]">名称</TableHead>
                    <TableHead className="min-w-[140px]">Active 域名</TableHead>
                    <TableHead className="min-w-[100px]">Issuer</TableHead>
                    <TableHead className="text-center">Server</TableHead>
                    <TableHead>有效性</TableHead>
                    <TableHead>可下载</TableHead>
                    <TableHead className="min-w-[130px]">到期</TableHead>
                    <TableHead>自动续期</TableHead>
                    <TableHead className="min-w-[140px]">最近 Operation</TableHead>
                    <TableHead className="w-[160px]">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filtered.map((c) => (
                    <ManagedCertificateRow
                      key={c.id}
                      cert={c}
                      issuerName={issuerNames.get(c.certificate_issuer_id) ?? c.certificate_issuer_id}
                    />
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      <ManagedCertificateForm open={formOpen} onOpenChange={setFormOpen} />
    </div>
  )
}

function ManagedCertificateRow({
  cert,
  issuerName,
}: {
  cert: ManagedCertificate
  issuerName: string
}) {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const domains = activeDomains(cert)
  const staging = cert.active_version?.staging_untrusted
  const op = cert.latest_operation

  const issueRunning = op?.type === "Issue" && operationInFlight(op)
  const renewRunning = op?.type === "Renew" && operationInFlight(op)
  const revokeRunning = op?.type === "Revoke" && operationInFlight(op)
  const anyOpRunning = issueRunning || renewRunning || revokeRunning

  const canIssue =
    user &&
    (cert.has_unpublished_desired_changes || cert.active_version) &&
    !issueRunning &&
    !renewRunning
  const canRenew = user && cert.active_version && !issueRunning && !renewRunning
  const canRetryFailed = user && op?.status === "Failed" && !anyOpRunning

  const invalidateQueries = () => {
    queryClient.invalidateQueries({ queryKey: certQueryKeys.detail(cert.id) })
    queryClient.invalidateQueries({ queryKey: certQueryKeys.list })
  }

  const issueMutation = useMutation({
    mutationFn: () => api.submitIssueOperation(cert.id),
    onSuccess: () => {
      toast.success("已提交签发（使用 desired config）")
      invalidateQueries()
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "签发提交失败，请检查 Issuer 与 DNS 配置")
    },
  })

  const renewMutation = useMutation({
    mutationFn: () => api.submitRenewOperation(cert.id),
    onSuccess: () => {
      toast.success("已提交续期（使用 active 签发快照）")
      invalidateQueries()
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "续期提交失败，请稍后重试")
    },
  })

  const retryMutation = useMutation({
    mutationFn: async () =>
      op?.type === "Renew"
        ? api.submitRenewOperation(cert.id)
        : api.submitIssueOperation(cert.id),
    onSuccess: () => {
      toast.success("已重新提交失败 operation")
      invalidateQueries()
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "重试失败，请稍后重试")
    },
  })

  return (
    <TableRow>
      <TableCell className="font-medium">
        <div className="flex min-w-0 flex-col gap-1">
          <span className="truncate">{cert.name}</span>
          {staging ? <CertificateStagingBadge /> : null}
        </div>
      </TableCell>
      <TableCell>
        <DomainsCell domains={domains} />
      </TableCell>
      <TableCell className="max-w-[120px] truncate text-sm" title={issuerName}>
        {issuerName}
      </TableCell>
      <TableCell className="text-center tabular-nums">
        {(cert.server_ids ?? []).length}
      </TableCell>
      <TableCell>
        <CertificateValidityBadge validity={cert.active_validity} />
      </TableCell>
      <TableCell>
        <CertificateAvailabilityBadge available={cert.bundle_available} />
      </TableCell>
      <TableCell className="text-sm whitespace-nowrap">
        {formatManagedCertExpiry(cert.active_version?.not_after)}
      </TableCell>
      <TableCell>
        <AutoRenewCell enabled={cert.auto_renew_enabled} />
      </TableCell>
      <TableCell>
        <CertificateOperationBadge operation={cert.latest_operation} />
      </TableCell>
      <TableCell>
        <div className="flex items-center justify-end gap-1">
          {canIssue ? (
            <Button
              variant="ghost"
              size="icon"
              title="签发新版本（发布 desired config）"
              disabled={issueMutation.isPending}
              onClick={() => issueMutation.mutate()}
            >
              {issueMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <Rocket className="size-4" />}
            </Button>
          ) : null}
          {canRenew ? (
            <Button
              variant="ghost"
              size="icon"
              title="续期 active 版本"
              disabled={renewMutation.isPending}
              onClick={() => renewMutation.mutate()}
            >
              {renewMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <Repeat className="size-4" />}
            </Button>
          ) : null}
          {canRetryFailed ? (
            <Button
              variant="ghost"
              size="icon"
              title={`重试失败的 ${op?.type} operation`}
              disabled={retryMutation.isPending}
              onClick={() => retryMutation.mutate()}
            >
              {retryMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : <RotateCcw className="size-4" />}
            </Button>
          ) : null}
          <Button asChild variant="ghost" size="sm">
            <Link to={`/certificates/managed/${cert.id}`}>详情</Link>
          </Button>
        </div>
      </TableCell>
    </TableRow>
  )
}
