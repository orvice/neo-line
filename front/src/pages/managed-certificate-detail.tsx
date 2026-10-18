import { useState, type ReactNode } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  ChevronLeft,
  Download,
  Loader2,
  Pencil,
  RefreshCw,
  Repeat,
  Rocket,
  RotateCcw,
  ShieldOff,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type {
  CertificateKeyType,
  CertificateRevocationReason,
  CertificateVersionMetadata,
  ManagedCertificate,
} from "@/lib/types"
import { useAuth } from "@/lib/auth"
import {
  certQueryKeys,
  operationInFlight,
  validityDescriptions,
} from "@/lib/certificate-ui"
import { formatManagedCertExpiry, formatTime } from "@/lib/format"
import { CertificateNavTabs } from "@/components/certificate-nav-tabs"
import {
  CertificateAvailabilityBadge,
  CertificateOperationBadge,
  CertificateStagingBadge,
  CertificateValidityBadge,
} from "@/components/certificate-status-badges"
import { ManagedCertificateForm } from "@/components/managed-certificate-form"
import { ManagedCertificateServerAssignment } from "@/components/managed-certificate-server-assignment"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

const KEY_LABELS: Record<CertificateKeyType, string> = {
  ec_p256: "EC P-256",
  rsa_2048: "RSA-2048",
  unspecified: "—",
}

const REVOKE_REASON_LABELS: Record<CertificateRevocationReason, string> = {
  unspecified: "未指定 (unspecified)",
  key_compromise: "密钥泄露 (keyCompromise)",
  ca_compromise: "CA 泄露 (cACompromise)",
  affiliation_changed: "隶属关系变更 (affiliationChanged)",
  superseded: "已被替代 (superseded)",
  cessation_of_operation: "停止运营 (cessationOfOperation)",
  certificate_hold: "证书挂起 (certificateHold)",
  privilege_withdrawn: "权限撤销 (privilegeWithdrawn)",
  aa_compromise: "AA 泄露 (aACompromise)",
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

function DetailSection({
  title,
  description,
  children,
  className,
}: {
  title: string
  description?: string
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cn("border-b py-6 last:border-b-0", className)}>
      <div className="mb-4">
        <h2 className="text-base font-semibold">{title}</h2>
        {description ? (
          <p className="text-muted-foreground mt-1 text-sm">{description}</p>
        ) : null}
      </div>
      {children}
    </section>
  )
}

function VersionSection({
  title,
  version,
  certName,
  versionSlot,
  certId,
  readOnly,
  onActivate,
  activatePending,
  onRevoke,
  revokePending,
}: {
  title: string
  version?: CertificateVersionMetadata
  certName: string
  versionSlot: "active" | "previous"
  certId: string
  readOnly: boolean
  onActivate?: () => void
  activatePending?: boolean
  onRevoke?: () => void
  revokePending?: boolean
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
      toast.error(err instanceof ApiError ? err.message : "下载失败，请稍后重试")
    },
  })

  if (!version) {
    return (
      <DetailSection title={title}>
        <p className="text-muted-foreground text-sm">暂无版本</p>
      </DetailSection>
    )
  }

  const expired = versionExpired(version)
  const revoked = Boolean(version.revoked_at)
  const pendingRevoke = Boolean(version.revoke_pending)

  return (
    <DetailSection title={title}>
      <dl className="grid gap-3 text-sm sm:grid-cols-2">
        <Field label="版本 ID" mono>
          {version.id}
        </Field>
        <Field label="指纹 (SHA-256)" mono>
          {version.leaf_fingerprint}
        </Field>
        <Field label="Serial" mono>
          {version.serial_number}
        </Field>
        {version.not_before && version.not_after ? (
          <Field label="有效期">
            {formatTime(version.not_before)} — {formatTime(version.not_after)}
            {expired ? <span className="ml-2 text-amber-700">（已过期）</span> : null}
            {revoked ? <span className="ml-2 text-destructive">（已吊销）</span> : null}
            {pendingRevoke ? (
              <span className="ml-2 text-destructive">（吊销处理中，已停止分发）</span>
            ) : null}
          </Field>
        ) : null}
        {version.config_snapshot ? (
          <Field label="签发快照域名" mono className="sm:col-span-2">
            {(version.config_snapshot.domains ?? []).join(", ")}
          </Field>
        ) : null}
        {version.staging_untrusted ? (
          <div className="sm:col-span-2">
            <CertificateStagingBadge />
          </div>
        ) : null}
      </dl>
      {!readOnly && !revoked && !pendingRevoke ? (
        <div className="mt-4 flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={downloadBundle.isPending}
            onClick={() => downloadBundle.mutate()}
          >
            {downloadBundle.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Download className="size-4" />
            )}
            {downloadBundle.isPending ? "下载中…" : "下载 PEM bundle"}
          </Button>
          {onRevoke ? (
            <Button
              variant="destructive"
              size="sm"
              disabled={revokePending}
              onClick={onRevoke}
            >
              <ShieldOff className="size-4" />
              吊销此版本
            </Button>
          ) : null}
          {versionSlot === "previous" && onActivate ? (
            <Button
              variant="secondary"
              size="sm"
              disabled={activatePending}
              onClick={onActivate}
            >
              {activatePending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : null}
              激活 previous 版本
            </Button>
          ) : null}
        </div>
      ) : null}
    </DetailSection>
  )
}

function Field({
  label,
  children,
  mono,
  className,
}: {
  label: string
  children: ReactNode
  mono?: boolean
  className?: string
}) {
  return (
    <div className={className}>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn("mt-0.5 break-all", mono && "font-mono text-xs")}>{children}</dd>
    </div>
  )
}

export function ManagedCertificateDetailPage() {
  const { user } = useAuth()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { certId } = useParams<{ certId: string }>()
  const id = certId ?? ""
  const [formOpen, setFormOpen] = useState(false)
  const [activateConfirmOpen, setActivateConfirmOpen] = useState(false)
  const [revokeConfirmOpen, setRevokeConfirmOpen] = useState(false)
  const [revokeTarget, setRevokeTarget] = useState<{
    versionId: string
    slot: "active" | "previous"
  } | null>(null)
  const [revokeReason, setRevokeReason] =
    useState<CertificateRevocationReason>("unspecified")
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)

  const certQuery = useQuery({
    queryKey: certQueryKeys.detail(id),
    queryFn: () => api.getManagedCertificate(id),
    enabled: Boolean(id),
    refetchInterval: (q) => {
      const op = q.state.data?.certificate.latest_operation
      return operationInFlight(op) ? 3000 : false
    },
  })

  const issuerQuery = useQuery({
    queryKey: certQueryKeys.issuers,
    queryFn: () => api.listCertificateIssuers({ page_size: 200 }),
    enabled: Boolean(id),
  })
  const dnsQuery = useQuery({
    queryKey: certQueryKeys.dnsAccounts,
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

  function invalidateCertQueries() {
    queryClient.invalidateQueries({ queryKey: certQueryKeys.detail(id) })
    queryClient.invalidateQueries({ queryKey: certQueryKeys.list })
  }

  const issueMutation = useMutation({
    mutationFn: () => api.submitIssueOperation(id),
    onSuccess: () => {
      toast.success("已提交签发新版本（使用 desired config）")
      invalidateCertQueries()
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "签发提交失败，请检查 Issuer 与 DNS 配置")
    },
  })

  const renewMutation = useMutation({
    mutationFn: () => api.submitRenewOperation(id),
    onSuccess: () => {
      toast.success("已提交续期（使用 active 签发快照）")
      invalidateCertQueries()
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "续期提交失败，请稍后重试")
    },
  })

  const retryMutation = useMutation({
    mutationFn: async () => {
      if (!op || op.status !== "Failed") {
        throw new Error("当前没有可重试的失败 operation")
      }
      if (op.type === "Renew") {
        return api.submitRenewOperation(id)
      }
      return api.submitIssueOperation(id)
    },
    onSuccess: () => {
      toast.success("已重新提交失败 operation")
      invalidateCertQueries()
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "重试失败，请稍后重试")
    },
  })

  const activateMutation = useMutation({
    mutationFn: () =>
      api.activatePreviousVersion(id, cert!.previous_version!.id),
    onSuccess: () => {
      toast.success("已激活 previous 版本")
      setActivateConfirmOpen(false)
      invalidateCertQueries()
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "激活失败，请确认版本未被吊销")
    },
  })

  const revokeMutation = useMutation({
    mutationFn: () =>
      api.submitRevokeVersion(id, revokeTarget!.versionId, revokeReason),
    onSuccess: () => {
      toast.success("吊销请求已接受；该版本已立即停止分发")
      setRevokeConfirmOpen(false)
      setRevokeTarget(null)
      invalidateCertQueries()
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "吊销提交失败，请稍后重试")
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteManagedCertificate(id),
    onSuccess: () => {
      toast.success("已删除托管证书（本地记录；未向 CA 隐式吊销）")
      queryClient.invalidateQueries({ queryKey: certQueryKeys.list })
      navigate("/certificates/managed")
    },
    onError: (err: unknown) => {
      toast.error(err instanceof ApiError ? err.message : "删除失败，请解除 Server 分配并等待 operation 结束")
    },
  })

  const issueRunning = op?.type === "Issue" && operationInFlight(op)
  const renewRunning = op?.type === "Renew" && operationInFlight(op)
  const revokeRunning = op?.type === "Revoke" && operationInFlight(op)
  const anyOpRunning = issueRunning || renewRunning || revokeRunning
  const canRetryFailed = op?.status === "Failed" && !anyOpRunning

  const canDelete =
    user &&
    cert &&
    (cert.server_ids ?? []).length === 0 &&
    !anyOpRunning

  const canIssue =
    user &&
    cert &&
    (cert.has_unpublished_desired_changes || cert.active_version) &&
    !issueRunning &&
    !renewRunning

  const canRenew = user && cert?.active_version && !issueRunning && !renewRunning

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
              : "加载证书详情失败"}
          </div>
        ) : cert ? (
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="text-2xl font-semibold">{cert.name}</h1>
                <CertificateValidityBadge validity={cert.active_validity} />
                {cert.active_version?.staging_untrusted ? (
                  <CertificateStagingBadge />
                ) : null}
              </div>
              <p className="text-muted-foreground font-mono text-xs">{cert.id}</p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
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
                  {canIssue ? (
                    <Button
                      disabled={issueMutation.isPending}
                      onClick={() => issueMutation.mutate()}
                      title="发布 desired config 并签发新版本"
                    >
                      {issueMutation.isPending ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <Rocket className="size-4" />
                      )}
                      {issueMutation.isPending ? "提交中…" : "签发新版本"}
                    </Button>
                  ) : null}
                  {canRenew ? (
                    <Button
                      variant="secondary"
                      disabled={renewMutation.isPending}
                      onClick={() => renewMutation.mutate()}
                      title="按 active 签发快照续期（与 desired 无关）"
                    >
                      {renewMutation.isPending ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <Repeat className="size-4" />
                      )}
                      {renewMutation.isPending ? "提交中…" : "续期 active"}
                    </Button>
                  ) : null}
                  {canRetryFailed ? (
                    <Button
                      variant="outline"
                      disabled={retryMutation.isPending}
                      onClick={() => retryMutation.mutate()}
                      title={`重试失败的 ${op?.type} operation`}
                    >
                      {retryMutation.isPending ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <RotateCcw className="size-4" />
                      )}
                      {retryMutation.isPending ? "重试中…" : "重试失败 operation"}
                    </Button>
                  ) : null}
                  <Button variant="outline" onClick={() => setFormOpen(true)}>
                    <Pencil className="size-4" />
                    编辑
                  </Button>
                  {canDelete ? (
                    <Button
                      variant="destructive"
                      onClick={() => setDeleteConfirmOpen(true)}
                    >
                      <Trash2 className="size-4" />
                      删除证书
                    </Button>
                  ) : null}
                </>
              ) : null}
            </div>
          </div>
        ) : null}
      </div>

      {cert ? (
        <div className="rounded-lg border bg-card px-4 sm:px-6">
          <DetailSection
            title="Active 状态"
            description={validityDescriptions[cert.active_validity]}
          >
            <div className="flex flex-wrap items-center gap-3 text-sm">
              <CertificateValidityBadge validity={cert.active_validity} />
              <CertificateAvailabilityBadge available={cert.bundle_available} />
              <span className="text-muted-foreground">
                到期：{formatManagedCertExpiry(cert.active_version?.not_after)}
              </span>
            </div>
            {cert.active_validity === "Missing" && !cert.bundle_available ? (
              <p className="text-muted-foreground mt-3 text-xs">
                首次签发尚未完成；Pending Issue operation 由后台 reconciler 执行。
              </p>
            ) : null}
          </DetailSection>

          <DetailSection
            title="Desired / Active 差异"
            description="修改签发字段只更新 desired config；点击「签发新版本」才会发布。"
          >
            {cert.has_unpublished_desired_changes && diffLines.length > 0 ? (
              <div className="rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-900 dark:text-amber-200">
                <p className="mb-1 font-medium">与 active 签发快照存在差异（尚未发布）：</p>
                <ul className="list-inside list-disc space-y-0.5">
                  {diffLines.map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
              </div>
            ) : cert.active_version ? (
              <p className="text-muted-foreground text-sm">
                当前 desired config 与 active 签发快照一致。
              </p>
            ) : (
              <p className="text-muted-foreground text-sm">
                尚无 active 版本；desired config 将在首次签发成功后成为 active 快照。
              </p>
            )}
            <dl className="mt-4 grid gap-3 text-sm sm:grid-cols-2">
              <Field label="Desired 域名" mono className="sm:col-span-2">
                {cert.domains.map((d, i) => (
                  <div key={d}>
                    {i === 0 ? "★ " : "  "}
                    {d}
                    {i === 0 ? "（主域名）" : ""}
                  </div>
                ))}
              </Field>
              <Field label="ACME Issuer">{issuerName}</Field>
              <Field label="DNS 账户">{dnsName}</Field>
              <Field label="密钥类型">{KEY_LABELS[cert.key_type] ?? cert.key_type}</Field>
            </dl>
          </DetailSection>

          <VersionSection
            title="Active 版本"
            version={cert.active_version}
            certName={cert.name}
            versionSlot="active"
            certId={id}
            readOnly={!user}
            revokePending={revokeMutation.isPending}
            onRevoke={
              cert.active_version &&
              !cert.active_version.revoked_at &&
              !cert.active_version.revoke_pending &&
              !anyOpRunning
                ? () => {
                    setRevokeTarget({
                      versionId: cert.active_version!.id,
                      slot: "active",
                    })
                    setRevokeReason("unspecified")
                    setRevokeConfirmOpen(true)
                  }
                : undefined
            }
          />

          <VersionSection
            title="Previous 版本"
            version={cert.previous_version}
            certName={cert.name}
            versionSlot="previous"
            certId={id}
            readOnly={!user}
            activatePending={activateMutation.isPending}
            revokePending={revokeMutation.isPending}
            onRevoke={
              cert.previous_version &&
              !cert.previous_version.revoked_at &&
              !cert.previous_version.revoke_pending &&
              !anyOpRunning
                ? () => {
                    setRevokeTarget({
                      versionId: cert.previous_version!.id,
                      slot: "previous",
                    })
                    setRevokeReason("unspecified")
                    setRevokeConfirmOpen(true)
                  }
                : undefined
            }
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

          <DetailSection title="最新 Operation">
            {!op ? (
              <p className="text-muted-foreground text-sm">暂无 operation 记录</p>
            ) : (
              <>
                <div className="mb-3">
                  <CertificateOperationBadge operation={op} />
                </div>
                <dl className="grid gap-3 text-sm sm:grid-cols-2">
                  <Field label="尝试次数">{op.attempt_count}</Field>
                  {op.config_snapshot ? (
                    <Field label="配置快照" mono className="sm:col-span-2">
                      {op.config_snapshot.domains.join(", ")}
                    </Field>
                  ) : null}
                  {op.error_summary ? (
                    <Field label="错误摘要" className="sm:col-span-2">
                      <span className="text-destructive">{op.error_summary}</span>
                    </Field>
                  ) : null}
                  {op.warning ? (
                    <Field label="告警" className="sm:col-span-2">
                      <span className="text-amber-700">{op.warning}</span>
                    </Field>
                  ) : null}
                  {op.started_at ? (
                    <Field label="开始时间">{formatTime(op.started_at)}</Field>
                  ) : null}
                  {op.finished_at ? (
                    <Field label="结束时间">{formatTime(op.finished_at)}</Field>
                  ) : null}
                  {op.next_attempt_at && op.status === "Pending" ? (
                    <Field label="下次自动重试">{formatTime(op.next_attempt_at)}</Field>
                  ) : null}
                  <Field label="创建时间">{formatTime(op.created_at)}</Field>
                </dl>
              </>
            )}
          </DetailSection>

          <DetailSection
            title="续期策略"
            description="自动续期使用 active 签发快照；未发布的 desired config 不会被自动续期采用。"
          >
            <dl className="grid gap-3 text-sm sm:grid-cols-2">
              <Field label="自动续期">{cert.auto_renew_enabled ? "开启" : "关闭"}</Field>
              <Field label="续期提前（配置）">{cert.renew_before_days} 天</Field>
              {cert.active_version ? (
                <>
                  <Field label="有效续期窗口">
                    {cert.effective_renewal_window_days ?? cert.renew_before_days} 天
                    <span className="text-muted-foreground ml-1 text-xs">
                      （min(配置, 有效期/3)）
                    </span>
                  </Field>
                  <Field label="下次自动续期">
                    {cert.auto_renew_enabled ? (
                      cert.next_renewal_at ? (
                        formatTime(cert.next_renewal_at)
                      ) : (
                        "—"
                      )
                    ) : (
                      <span className="text-muted-foreground">已关闭自动续期</span>
                    )}
                  </Field>
                </>
              ) : null}
            </dl>
          </DetailSection>

          <DetailSection title="通知组">
            <p className="text-sm">
              {(cert.notify_group_ids ?? []).length === 0
                ? "（无）"
                : (cert.notify_group_ids ?? [])
                    .map(
                      (nid) => notifyGroups.find((g) => g.id === nid)?.name ?? nid
                    )
                    .join("、")}
            </p>
            {cert.last_notification_warning ? (
              <div className="mt-3 text-sm">
                <p className="text-muted-foreground">最近通知告警</p>
                <p className="text-amber-700">{cert.last_notification_warning}</p>
                {cert.last_notification_warning_at ? (
                  <p className="text-muted-foreground text-xs">
                    {formatTime(cert.last_notification_warning_at)}
                  </p>
                ) : null}
              </div>
            ) : null}
          </DetailSection>

          <ManagedCertificateServerAssignment
            certificate={cert}
            readOnly={!user}
            variant="flat"
          />

          <DetailSection title="元数据">
            <p className="text-muted-foreground text-xs">
              创建于 {formatTime(cert.created_at)} · 更新于 {formatTime(cert.updated_at)}
            </p>
            {user && (cert.server_ids ?? []).length > 0 ? (
              <p className="mt-2 text-sm text-amber-700">
                删除前须解除全部 Server 分配且无运行中 operation。
              </p>
            ) : null}
          </DetailSection>
        </div>
      ) : null}

      {cert ? (
        <ManagedCertificateForm
          open={formOpen}
          onOpenChange={setFormOpen}
          certificate={cert}
        />
      ) : null}

      <ConfirmDialog
        open={activateConfirmOpen}
        onOpenChange={setActivateConfirmOpen}
        title="激活已过期的 previous 版本？"
        description="该 previous 版本证书已过期。激活后 Server 将下载过期 bundle；请确认这是有意的灾难恢复操作。Desired config 不会被修改。"
        confirmText="确认激活 previous"
        pending={activateMutation.isPending}
        onConfirm={() => activateMutation.mutate()}
      />

      <ConfirmDialog
        open={revokeConfirmOpen}
        onOpenChange={setRevokeConfirmOpen}
        title={`吊销 ${revokeTarget?.slot === "active" ? "active" : "previous"} 版本？`}
        description={
          <div className="flex flex-col gap-3 text-sm">
            <p>
              吊销请求一经接受，该版本将<strong>立即停止分发</strong>（不等待 CA
              确认）。CA 调用失败时仍保持阻止并自动重试。
            </p>
            <p>
              吊销 active <strong>不会</strong>自动激活 previous 或签发新版本；后续动作须由
              Admin 明确选择。此操作向 CA 提交不可逆的吊销请求。
            </p>
            <label className="flex flex-col gap-1">
              <span className="text-muted-foreground">RFC 5280 吊销原因</span>
              <select
                className="border-input bg-background rounded-md border px-2 py-1.5 text-sm"
                value={revokeReason}
                onChange={(e) =>
                  setRevokeReason(e.target.value as CertificateRevocationReason)
                }
              >
                {(Object.keys(REVOKE_REASON_LABELS) as CertificateRevocationReason[]).map(
                  (key) => (
                    <option key={key} value={key}>
                      {REVOKE_REASON_LABELS[key]}
                    </option>
                  )
                )}
              </select>
            </label>
          </div>
        }
        confirmText="确认吊销"
        pending={revokeMutation.isPending}
        onConfirm={() => revokeMutation.mutate()}
      />

      <ConfirmDialog
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
        title="删除托管证书？"
        description="此操作仅删除 neo-line 中的 desired config、active/previous 版本、operations 与通知节流状态；不会向 CA 隐式吊销证书。audit_logs 与 Server 的 CertificateAccessToken 不会被删除。此操作不可恢复。"
        confirmText="确认删除本地记录"
        pending={deleteMutation.isPending}
        onConfirm={() => deleteMutation.mutate()}
      />
    </div>
  )
}
