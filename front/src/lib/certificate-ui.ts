import type {
  CertificateOperation,
  CertificateOperationStatus,
  CertificateOperationType,
  CertificateValidity,
  ManagedCertificate,
} from "./types"
import { isZeroTime } from "./format"

export const certQueryKeys = {
  list: ["managed-certificates"] as const,
  detail: (id: string) => ["managed-certificate", id] as const,
  issuers: ["certificate-issuers"] as const,
  dnsAccounts: ["dns-provider-accounts"] as const,
  tokens: (serverId: string) => ["certificate-access-tokens", serverId] as const,
}

export const validityLabels: Record<CertificateValidity, string> = {
  Missing: "Missing",
  Valid: "Valid",
  RenewalDue: "RenewalDue",
  Expired: "Expired",
  Revoked: "Revoked",
  Unspecified: "—",
}

export const validityDescriptions: Record<CertificateValidity, string> = {
  Missing: "尚无可用 active 版本",
  Valid: "证书有效",
  RenewalDue: "进入续期窗口",
  Expired: "证书已过期",
  Revoked: "证书已吊销",
  Unspecified: "未知",
}

export const opStatusLabels: Record<CertificateOperationStatus, string> = {
  Pending: "Pending",
  Running: "Running",
  Succeeded: "Succeeded",
  Failed: "Failed",
  Unspecified: "—",
}

export const opTypeLabels: Record<CertificateOperationType, string> = {
  Issue: "Issue",
  Renew: "Renew",
  Revoke: "Revoke",
  Unspecified: "—",
}

export type ManagedCertListFilter =
  | { kind: "all" }
  | { kind: "validity"; value: CertificateValidity }
  | { kind: "expiring"; days: number }
  | { kind: "opFailed" }

export function parseManagedCertListFilter(
  params: URLSearchParams
): ManagedCertListFilter {
  const validity = params.get("validity")
  if (
    validity === "Missing" ||
    validity === "Valid" ||
    validity === "RenewalDue" ||
    validity === "Expired" ||
    validity === "Revoked"
  ) {
    return { kind: "validity", value: validity }
  }
  const expiring = params.get("expiring")
  if (expiring) {
    const days = Number.parseInt(expiring, 10)
    if (!Number.isNaN(days) && days > 0) {
      return { kind: "expiring", days }
    }
  }
  if (params.get("op") === "failed") {
    return { kind: "opFailed" }
  }
  return { kind: "all" }
}

export function managedCertListFilterLabel(filter: ManagedCertListFilter): string {
  switch (filter.kind) {
    case "validity":
      return `有效性：${validityLabels[filter.value]}`
    case "expiring":
      return `${filter.days} 天内到期`
    case "opFailed":
      return "最近 operation 失败"
    default:
      return "全部证书"
  }
}

export function managedCertListFilterHref(filter: ManagedCertListFilter): string {
  switch (filter.kind) {
    case "validity":
      return `/certificates/managed?validity=${filter.value}`
    case "expiring":
      return `/certificates/managed?expiring=${filter.days}`
    case "opFailed":
      return "/certificates/managed?op=failed"
    default:
      return "/certificates/managed"
  }
}

export function daysUntilNotAfter(notAfter?: string): number | undefined {
  if (isZeroTime(notAfter)) return undefined
  const end = new Date(notAfter as string).getTime()
  if (Number.isNaN(end)) return undefined
  return Math.ceil((end - Date.now()) / (24 * 60 * 60 * 1000))
}

export function activeDomains(cert: ManagedCertificate): string[] {
  return cert.active_version?.config_snapshot?.domains?.length
    ? cert.active_version.config_snapshot.domains
    : cert.domains
}

export function matchesManagedCertFilter(
  cert: ManagedCertificate,
  filter: ManagedCertListFilter
): boolean {
  switch (filter.kind) {
    case "validity":
      return cert.active_validity === filter.value
    case "expiring": {
      const days = daysUntilNotAfter(cert.active_version?.not_after)
      if (days === undefined) return false
      return days >= 0 && days <= filter.days
    }
    case "opFailed":
      return cert.latest_operation?.status === "Failed"
    default:
      return true
  }
}

export function summarizeManagedCertificates(certs: ManagedCertificate[]) {
  let failedOps = 0
  let renewalDue = 0
  let expiring7 = 0
  let expired = 0

  for (const cert of certs) {
    if (cert.latest_operation?.status === "Failed") failedOps += 1
    if (cert.active_validity === "RenewalDue") renewalDue += 1
    if (cert.active_validity === "Expired") expired += 1
    const days = daysUntilNotAfter(cert.active_version?.not_after)
    if (days !== undefined && days >= 0 && days <= 7) expiring7 += 1
  }

  return {
    total: certs.length,
    failedOps,
    renewalDue,
    expiring7,
    expired,
  }
}

export function operationInFlight(op?: CertificateOperation): boolean {
  return op?.status === "Pending" || op?.status === "Running"
}

export function latestOperationSummary(op?: CertificateOperation): string {
  if (!op) return "—"
  return `${opTypeLabels[op.type]} · ${opStatusLabels[op.status]}`
}
