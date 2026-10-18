import type {
  CertificateIssuerRegistrationStatus,
  CertificateOperation,
  CertificateOperationStatus,
  CertificateOperationType,
  CertificateValidity,
} from "@/lib/types"
import {
  opStatusLabels,
  opTypeLabels,
  validityLabels,
} from "@/lib/certificate-ui"
import { cn } from "@/lib/utils"

const badgeBase =
  "inline-flex max-w-full items-center truncate rounded-full px-2 py-0.5 text-xs font-medium"

export function CertificateValidityBadge({
  validity,
  className,
}: {
  validity: CertificateValidity
  className?: string
}) {
  const tone = validityTone(validity)
  return (
    <span className={cn(badgeBase, tone, className)} title={validityLabels[validity]}>
      {validityLabels[validity]}
    </span>
  )
}

export function CertificateOperationBadge({
  operation,
  className,
}: {
  operation?: CertificateOperation
  className?: string
}) {
  if (!operation) {
    return <span className="text-muted-foreground text-sm">—</span>
  }
  const tone = opStatusTone(operation.status)
  const label = `${opTypeLabels[operation.type]} · ${opStatusLabels[operation.status]}`
  return (
    <span className={cn(badgeBase, tone, className)} title={label}>
      {label}
    </span>
  )
}

export function CertificateAvailabilityBadge({
  available,
  className,
}: {
  available: boolean
  className?: string
}) {
  return available ? (
    <span
      className={cn(
        badgeBase,
        "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300",
        className
      )}
    >
      可下载
    </span>
  ) : (
    <span
      className={cn(
        badgeBase,
        "bg-muted text-muted-foreground",
        className
      )}
    >
      不可下载
    </span>
  )
}

export function CertificateStagingBadge({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        badgeBase,
        "bg-amber-500/15 text-amber-800 dark:text-amber-300",
        className
      )}
      title="staging 证书不受公共客户端信任"
    >
      staging / 不受信任
    </span>
  )
}

export function IssuerRegistrationBadge({
  status,
  className,
}: {
  status: CertificateIssuerRegistrationStatus
  className?: string
}) {
  const tone = issuerStatusTone(status)
  return (
    <span className={cn(badgeBase, tone, className)} title={status}>
      {status}
    </span>
  )
}

function validityTone(validity: CertificateValidity): string {
  switch (validity) {
    case "Missing":
      return "bg-muted text-muted-foreground"
    case "Valid":
      return "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300"
    case "RenewalDue":
      return "bg-amber-500/15 text-amber-700 dark:text-amber-300"
    case "Expired":
      return "bg-orange-500/15 text-orange-700 dark:text-orange-300"
    case "Revoked":
      return "bg-red-500/15 text-red-700 dark:text-red-300"
    default:
      return "bg-muted text-muted-foreground"
  }
}

function opStatusTone(status: CertificateOperationStatus): string {
  switch (status) {
    case "Pending":
      return "bg-amber-500/15 text-amber-700 dark:text-amber-300"
    case "Running":
      return "bg-blue-500/15 text-blue-700 dark:text-blue-300"
    case "Succeeded":
      return "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300"
    case "Failed":
      return "bg-red-500/15 text-red-700 dark:text-red-300"
    default:
      return "bg-muted text-muted-foreground"
  }
}

function issuerStatusTone(status: CertificateIssuerRegistrationStatus): string {
  switch (status) {
    case "Ready":
      return "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300"
    case "Pending":
      return "bg-amber-500/15 text-amber-700 dark:text-amber-300"
    case "Failed":
      return "bg-red-500/15 text-red-700 dark:text-red-300"
    default:
      return "bg-muted text-muted-foreground"
  }
}

export type { CertificateOperationType, CertificateOperationStatus }
