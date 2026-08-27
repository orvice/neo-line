import { useState } from "react"
import { Link, useParams } from "react-router-dom"
import { useMutation, useQuery } from "@tanstack/react-query"
import { ChevronLeft, Download, Pencil, RefreshCw } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { CertificateKeyType } from "@/lib/types"
import { useAuth } from "@/lib/auth"
import { CertificateNavTabs } from "@/components/certificate-nav-tabs"
import { ManagedCertificateForm } from "@/components/managed-certificate-form"
import { ManagedCertificateServerAssignment } from "@/components/managed-certificate-server-assignment"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { formatTime } from "@/lib/format"

const KEY_LABELS: Record<CertificateKeyType, string> = {
  ec_p256: "EC P-256",
  rsa_2048: "RSA-2048",
  unspecified: "—",
}

const VALIDITY_LABELS: Record<string, string> = {
  Missing: "Missing",
  Valid: "Valid",
  RenewalDue: "RenewalDue",
  Expired: "Expired",
  Revoked: "Revoked",
  Unspecified: "—",
}

function downloadBytes(filename: string, bytes: Uint8Array) {
  const blob = new Blob([bytes], { type: "application/x-pem-file" })
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

export function ManagedCertificateDetailPage() {
  const { user } = useAuth()
  const { certId } = useParams<{ certId: string }>()
  const id = certId ?? ""
  const [formOpen, setFormOpen] = useState(false)

  const certQuery = useQuery({
    queryKey: ["managed-certificate", id],
    queryFn: () => api.getManagedCertificate(id),
    enabled: Boolean(id),
    refetchInterval: (q) => {
      const op = q.state.data?.certificate.latest_operation
      if (op?.status === "Pending" || op?.status === "Running") return 3000
      return false
    },
  })

  const issuerQuery = useQuery({
    queryKey: ["certificate-issuers"],
    queryFn: () => api.listCertificateIssuers({ page_size: 200 }),
    enabled: Boolean(id),
  })
  const dnsQuery = useQuery({
    queryKey: ["dns-provider-accounts"],
    queryFn: () => api.listDNSProviderAccounts({ page_size: 200 }),
    enabled: Boolean(id),
  })
  const notifyQuery = useQuery({
    queryKey: ["notify-groups"],
    queryFn: () => api.listNotifyGroups({ page_size: 200 }),
    enabled: Boolean(id),
  })

  const cert = certQuery.data?.certificate
  const issuers = issuerQuery.data?.issuers ?? []
  const dnsAccounts = dnsQuery.data?.accounts ?? []
  const notifyGroups = notifyQuery.data?.groups ?? []

  const issuerName =
    issuers.find((i) => i.id === cert?.certificate_issuer_id)?.name ??
    cert?.certificate_issuer_id
  const dnsName =
    dnsAccounts.find((a) => a.id === cert?.dns_provider_account_id)?.name ??
    cert?.dns_provider_account_id

  const op = cert?.latest_operation
  const active = cert?.active_version

  const downloadBundle = useMutation({
    mutationFn: () => api.getCertificateBundle(id),
    onSuccess: ({ bundle }) => {
      const prefix = cert?.name.replace(/[^\w.-]+/g, "_") ?? "cert"
      downloadBytes(`${prefix}-fullchain.pem`, bundle.fullchain_pem)
      downloadBytes(`${prefix}-private_key.pem`, bundle.private_key_pem)
      if (bundle.staging_untrusted) {
        toast.warning("已下载 staging 证书（不受公共信任）")
      } else {
        toast.success("证书 bundle 已下载")
      }
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "下载失败")
    },
  })

  return (
    <div className="animate-enter flex flex-col gap-6">
      <CertificateNavTabs />
      <div>
        <Button asChild variant="ghost" size="sm" className="mb-2 -ml-2">
          <Link to="/certificates/managed">
            <ChevronLeft className="size-4" />
            返回证书列表
          </Link>
        </Button>
        {certQuery.isLoading ? (
          <div className="text-muted-foreground">加载中…</div>
        ) : certQuery.isError ? (
          <div className="text-destructive">
            {certQuery.error instanceof ApiError
              ? certQuery.error.message
              : "加载失败"}
          </div>
        ) : cert ? (
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h1 className="text-2xl font-semibold">{cert.name}</h1>
              <p className="text-muted-foreground font-mono text-xs">{cert.id}</p>
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="icon"
                onClick={() => certQuery.refetch()}
                disabled={certQuery.isFetching}
                title="刷新"
              >
                <RefreshCw className={certQuery.isFetching ? "animate-spin" : ""} />
              </Button>
              {user ? (
                <Button variant="outline" onClick={() => setFormOpen(true)}>
                  <Pencil className="size-4" />
                  编辑
                </Button>
              ) : null}
            </div>
          </div>
        ) : null}
      </div>

      {cert && (
        <div className="grid gap-4 lg:grid-cols-2">
          <Card>
            <CardContent className="flex flex-col gap-3 pt-6 text-sm">
              <h2 className="font-semibold">Desired config</h2>
              <dl className="grid gap-2">
                <div>
                  <dt className="text-muted-foreground">域名</dt>
                  <dd className="font-mono text-xs break-all">
                    {cert.domains.map((d, i) => (
                      <div key={d}>
                        {i === 0 ? "★ " : "  "}
                        {d}
                        {i === 0 ? "（主域名）" : ""}
                      </div>
                    ))}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">ACME Issuer</dt>
                  <dd>{issuerName}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">DNS 账户</dt>
                  <dd>{dnsName}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">密钥类型</dt>
                  <dd>{KEY_LABELS[cert.key_type] ?? cert.key_type}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">自动续期</dt>
                  <dd>{cert.auto_renew_enabled ? "开启" : "关闭"}</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">续期提前</dt>
                  <dd>{cert.renew_before_days} 天</dd>
                </div>
                <div>
                  <dt className="text-muted-foreground">通知组</dt>
                  <dd>
                    {(cert.notify_group_ids ?? []).length === 0
                      ? "（无）"
                      : (cert.notify_group_ids ?? [])
                          .map(
                            (nid) =>
                              notifyGroups.find((g) => g.id === nid)?.name ?? nid
                          )
                          .join("、")}
                  </dd>
                </div>
              </dl>
            </CardContent>
          </Card>

          <div className="flex flex-col gap-4">
            <Card>
              <CardContent className="flex flex-col gap-2 pt-6 text-sm">
                <h2 className="font-semibold">Active 有效性</h2>
                <p>
                  状态：
                  <span className="font-medium">
                    {VALIDITY_LABELS[cert.active_validity] ?? cert.active_validity}
                  </span>
                  {active?.staging_untrusted ? (
                    <span className="ml-2 inline-flex rounded-full bg-amber-500/15 px-2 py-0.5 text-xs text-amber-800">
                      staging / 不受信任
                    </span>
                  ) : null}
                </p>
                <p>
                  Bundle 可下载：
                  {cert.bundle_available ? (
                    <span className="text-emerald-600">是</span>
                  ) : (
                    <span className="text-muted-foreground">否（尚无 active version）</span>
                  )}
                </p>
                {active && (
                  <dl className="mt-2 grid gap-2 border-t pt-2">
                    <div>
                      <dt className="text-muted-foreground">版本 ID</dt>
                      <dd className="font-mono text-xs">{active.id}</dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">指纹 (SHA-256)</dt>
                      <dd className="font-mono text-xs break-all">{active.leaf_fingerprint}</dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Serial</dt>
                      <dd className="font-mono text-xs">{active.serial_number}</dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Issuer CN</dt>
                      <dd>{active.issuer_common_name}</dd>
                    </div>
                    {active.not_before && active.not_after && (
                      <div>
                        <dt className="text-muted-foreground">有效期</dt>
                        <dd>
                          {formatTime(active.not_before)} — {formatTime(active.not_after)}
                        </dd>
                      </div>
                    )}
                  </dl>
                )}
                {cert.bundle_available && user ? (
                  <Button
                    variant="outline"
                    size="sm"
                    className="mt-2 w-fit"
                    disabled={downloadBundle.isPending}
                    onClick={() => downloadBundle.mutate()}
                  >
                    <Download className="size-4" />
                    下载 PEM bundle
                  </Button>
                ) : null}
                {cert.active_validity === "Missing" && !cert.bundle_available && (
                  <p className="text-muted-foreground text-xs">
                    首次签发尚未完成；Pending Issue operation 由后台 runner 执行。
                  </p>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardContent className="flex flex-col gap-2 pt-6 text-sm">
                <h2 className="font-semibold">最新 Operation</h2>
                {!op ? (
                  <p className="text-muted-foreground">暂无 operation 记录</p>
                ) : (
                  <dl className="grid gap-2">
                    <div className="flex flex-wrap gap-2">
                      <span className="inline-flex rounded-full bg-muted px-2 py-0.5 text-xs">
                        {op.type}
                      </span>
                      <span
                        className={`inline-flex rounded-full px-2 py-0.5 text-xs ${
                          op.status === "Pending"
                            ? "bg-amber-500/15 text-amber-700"
                            : op.status === "Running"
                              ? "bg-blue-500/15 text-blue-700"
                              : "bg-muted"
                        }`}
                      >
                        {op.status}
                      </span>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">尝试次数</dt>
                      <dd>{op.attempt_count}</dd>
                    </div>
                    {op.config_snapshot && (
                      <div>
                        <dt className="text-muted-foreground">配置快照</dt>
                        <dd className="font-mono text-xs break-all">
                          {op.config_snapshot.domains.join(", ")}
                        </dd>
                      </div>
                    )}
                    {op.error_summary && (
                      <div>
                        <dt className="text-muted-foreground">错误摘要</dt>
                        <dd className="text-destructive">{op.error_summary}</dd>
                      </div>
                    )}
                    {op.warning && (
                      <div>
                        <dt className="text-muted-foreground">告警</dt>
                        <dd className="text-amber-700">{op.warning}</dd>
                      </div>
                    )}
                    <div>
                      <dt className="text-muted-foreground">创建时间</dt>
                      <dd>{formatTime(op.created_at)}</dd>
                    </div>
                  </dl>
                )}
              </CardContent>
            </Card>

            <Card>
              <CardContent className="text-muted-foreground pt-6 text-xs">
                创建于 {formatTime(cert.created_at)} · 更新于{" "}
                {formatTime(cert.updated_at)}
              </CardContent>
            </Card>
          </div>
        </div>
      )}

      {cert && (
        <ManagedCertificateServerAssignment
          certificate={cert}
          readOnly={!user}
        />
      )}

      {cert && (
        <ManagedCertificateForm
          open={formOpen}
          onOpenChange={setFormOpen}
          certificate={cert}
        />
      )}
    </div>
  )
}
