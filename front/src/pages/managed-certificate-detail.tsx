import { useState } from "react"
import { Link, useParams } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ChevronLeft, Download, Pencil, RefreshCw, Repeat, Rocket } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type {
  CertificateKeyType,
  CertificateVersionMetadata,
  ManagedCertificate,
} from "@/lib/types"
import { useAuth } from "@/lib/auth"
import { CertificateNavTabs } from "@/components/certificate-nav-tabs"
import { ManagedCertificateForm } from "@/components/managed-certificate-form"
import { ManagedCertificateServerAssignment } from "@/components/managed-certificate-server-assignment"
import { ConfirmDialog } from "@/components/confirm-dialog"
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

function versionExpired(v: CertificateVersionMetadata | undefined): boolean {
  if (!v?.not_after) return false
  return new Date(v.not_after).getTime() < Date.now()
}

function desiredDiffLines(
  cert: ManagedCertificate,
  issuers: { id: string; name: string }[],
  dnsAccounts: { id: string; name: string }[]
): string[] {
  const snap = cert.active_version?.config_snapshot
  if (!snap) return []
  const lines: string[] = []
  const desiredDomains = cert.domains.join(", ")
  const activeDomains = (snap.domains ?? []).join(", ")
  if (desiredDomains !== activeDomains) {
    lines.push(`域名：active「${activeDomains}」→ desired「${desiredDomains}」`)
  }
  if (cert.certificate_issuer_id !== snap.certificate_issuer_id) {
    const from =
      issuers.find((i) => i.id === snap.certificate_issuer_id)?.name ??
      snap.certificate_issuer_id
    const to =
      issuers.find((i) => i.id === cert.certificate_issuer_id)?.name ??
      cert.certificate_issuer_id
    lines.push(`Issuer：${from} → ${to}`)
  }
  if (cert.dns_provider_account_id !== snap.dns_provider_account_id) {
    const from =
      dnsAccounts.find((a) => a.id === snap.dns_provider_account_id)?.name ??
      snap.dns_provider_account_id
    const to =
      dnsAccounts.find((a) => a.id === cert.dns_provider_account_id)?.name ??
      cert.dns_provider_account_id
    lines.push(`DNS 账户：${from} → ${to}`)
  }
  if (cert.key_type !== snap.key_type) {
    lines.push(
      `密钥类型：${KEY_LABELS[snap.key_type ?? "unspecified"] ?? snap.key_type} → ${KEY_LABELS[cert.key_type] ?? cert.key_type}`
    )
  }
  return lines
}

function VersionPanel({
  title,
  version,
  certName,
  versionSlot,
  certId,
  readOnly,
  onActivate,
  activatePending,
}: {
  title: string
  version?: CertificateVersionMetadata
  certName: string
  versionSlot: "active" | "previous"
  certId: string
  readOnly: boolean
  onActivate?: () => void
  activatePending?: boolean
}) {
  const downloadBundle = useMutation({
    mutationFn: () => api.getCertificateBundle(certId, versionSlot),
    onSuccess: ({ bundle }) => {
      const prefix = `${certName.replace(/[^\w.-]+/g, "_")}-${versionSlot}`
      downloadBytes(`${prefix}-fullchain.pem`, bundle.fullchain_pem)
      downloadBytes(`${prefix}-private_key.pem`, bundle.private_key_pem)
      toast.success(`${title} bundle 已下载（版本 ${bundle.version_id}）`)
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "下载失败")
    },
  })

  if (!version) {
    return (
      <Card>
        <CardContent className="flex flex-col gap-2 pt-6 text-sm">
          <h2 className="font-semibold">{title}</h2>
          <p className="text-muted-foreground">暂无版本</p>
        </CardContent>
      </Card>
    )
  }

  const expired = versionExpired(version)
  const revoked = Boolean(version.revoked_at)

  return (
    <Card>
      <CardContent className="flex flex-col gap-2 pt-6 text-sm">
        <h2 className="font-semibold">{title}</h2>
        <dl className="grid gap-2">
          <div>
            <dt className="text-muted-foreground">版本 ID</dt>
            <dd className="font-mono text-xs">{version.id}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">指纹 (SHA-256)</dt>
            <dd className="font-mono text-xs break-all">{version.leaf_fingerprint}</dd>
          </div>
          <div>
            <dt className="text-muted-foreground">Serial</dt>
            <dd className="font-mono text-xs">{version.serial_number}</dd>
          </div>
          {version.not_before && version.not_after && (
            <div>
              <dt className="text-muted-foreground">有效期</dt>
              <dd>
                {formatTime(version.not_before)} — {formatTime(version.not_after)}
                {expired ? (
                  <span className="ml-2 text-amber-700">（已过期）</span>
                ) : null}
                {revoked ? (
                  <span className="ml-2 text-destructive">（已吊销）</span>
                ) : null}
              </dd>
            </div>
          )}
          {version.config_snapshot && (
            <div>
              <dt className="text-muted-foreground">签发快照域名</dt>
              <dd className="font-mono text-xs break-all">
                {(version.config_snapshot.domains ?? []).join(", ")}
              </dd>
            </div>
          )}
        </dl>
        {!readOnly && !revoked ? (
          <div className="mt-2 flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={downloadBundle.isPending}
              onClick={() => downloadBundle.mutate()}
            >
              <Download className="size-4" />
              下载 PEM
            </Button>
            {versionSlot === "previous" && onActivate ? (
              <Button
                variant="secondary"
                size="sm"
                disabled={activatePending}
                onClick={onActivate}
              >
                激活此 previous 版本
              </Button>
            ) : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

export function ManagedCertificateDetailPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const { certId } = useParams<{ certId: string }>()
  const id = certId ?? ""
  const [formOpen, setFormOpen] = useState(false)
  const [activateConfirmOpen, setActivateConfirmOpen] = useState(false)

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
  const diffLines = cert ? desiredDiffLines(cert, issuers, dnsAccounts) : []

  const issueMutation = useMutation({
    mutationFn: () => api.submitIssueOperation(id),
    onSuccess: () => {
      toast.success("已提交签发新版本（使用 desired config）")
      queryClient.invalidateQueries({ queryKey: ["managed-certificate", id] })
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "提交失败")
    },
  })

  const renewMutation = useMutation({
    mutationFn: () => api.submitRenewOperation(id),
    onSuccess: () => {
      toast.success("已提交续期（使用 active 签发快照）")
      queryClient.invalidateQueries({ queryKey: ["managed-certificate", id] })
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "提交失败")
    },
  })

  const activateMutation = useMutation({
    mutationFn: () =>
      api.activatePreviousVersion(id, cert!.previous_version!.id),
    onSuccess: () => {
      toast.success("已激活 previous 版本")
      setActivateConfirmOpen(false)
      queryClient.invalidateQueries({ queryKey: ["managed-certificate", id] })
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "激活失败")
    },
  })

  const issueRunning =
    op?.type === "Issue" &&
    (op.status === "Pending" || op.status === "Running")
  const renewRunning =
    op?.type === "Renew" &&
    (op.status === "Pending" || op.status === "Running")

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
                <>
                  {cert.has_unpublished_desired_changes || cert.active_version ? (
                    <Button
                      variant="default"
                      disabled={issueRunning || renewRunning || issueMutation.isPending}
                      onClick={() => issueMutation.mutate()}
                      title="发布 desired config 并签发新版本"
                    >
                      <Rocket className="size-4" />
                      签发新版本
                    </Button>
                  ) : null}
                  {cert.active_version ? (
                    <Button
                      variant="secondary"
                      disabled={issueRunning || renewRunning || renewMutation.isPending}
                      onClick={() => renewMutation.mutate()}
                      title="按 active 签发快照续期（与 desired 无关）"
                    >
                      <Repeat className="size-4" />
                      续期 active
                    </Button>
                  ) : null}
                  <Button variant="outline" onClick={() => setFormOpen(true)}>
                    <Pencil className="size-4" />
                    编辑
                  </Button>
                </>
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
              {cert.has_unpublished_desired_changes && diffLines.length > 0 ? (
                <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-900">
                  <p className="mb-1 font-medium">与 active 签发快照存在差异（尚未发布）：</p>
                  <ul className="list-inside list-disc space-y-0.5">
                    {diffLines.map((line) => (
                      <li key={line}>{line}</li>
                    ))}
                  </ul>
                </div>
              ) : cert.active_version ? (
                <p className="text-muted-foreground text-xs">
                  当前 desired config 与 active 签发快照一致。
                </p>
              ) : null}
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
                  <dt className="text-muted-foreground">续期提前（配置）</dt>
                  <dd>{cert.renew_before_days} 天</dd>
                </div>
                {cert.active_version ? (
                  <>
                    <div>
                      <dt className="text-muted-foreground">有效续期窗口</dt>
                      <dd>
                        {cert.effective_renewal_window_days ?? cert.renew_before_days} 天
                        <span className="text-muted-foreground ml-1 text-xs">
                          （min(配置, 有效期/3)）
                        </span>
                      </dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">下次自动续期</dt>
                      <dd>
                        {cert.auto_renew_enabled ? (
                          cert.next_renewal_at ? (
                            formatTime(cert.next_renewal_at)
                          ) : (
                            "—"
                          )
                        ) : (
                          <span className="text-muted-foreground">已关闭自动续期</span>
                        )}
                      </dd>
                    </div>
                  </>
                ) : null}
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
                  {cert.active_version?.staging_untrusted ? (
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
                    <span className="text-muted-foreground">否</span>
                  )}
                </p>
                {cert.active_validity === "Missing" && !cert.bundle_available && (
                  <p className="text-muted-foreground text-xs">
                    首次签发尚未完成；Pending Issue operation 由后台 runner 执行。
                  </p>
                )}
              </CardContent>
            </Card>

            <VersionPanel
              title="Active 版本"
              version={cert.active_version}
              certName={cert.name}
              versionSlot="active"
              certId={id}
              readOnly={!user}
            />

            <VersionPanel
              title="Previous 版本"
              version={cert.previous_version}
              certName={cert.name}
              versionSlot="previous"
              certId={id}
              readOnly={!user}
              activatePending={activateMutation.isPending}
              onActivate={
                cert.previous_version && !cert.previous_version.revoked_at
                  ? () => {
                      if (versionExpired(cert.previous_version)) {
                        setActivateConfirmOpen(true)
                      } else {
                        activateMutation.mutate()
                      }
                    }
                  : undefined
              }
            />

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

      <ConfirmDialog
        open={activateConfirmOpen}
        onOpenChange={setActivateConfirmOpen}
        title="激活已过期的 previous 版本？"
        description="该 previous 版本证书已过期。激活后 Server 将下载过期 bundle；请确认这是有意的灾难恢复操作。Desired config 不会被修改。"
        confirmText="确认激活"
        pending={activateMutation.isPending}
        onConfirm={() => activateMutation.mutate()}
      />
    </div>
  )
}
