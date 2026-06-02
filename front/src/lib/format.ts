import type { CertificateInfo, HealthStatus } from "./types"

const ZERO_TIME_PREFIX = "0001-01-01"

export function isZeroTime(value?: string): boolean {
  return !value || value.startsWith(ZERO_TIME_PREFIX)
}

export function formatTime(value?: string): string {
  if (isZeroTime(value)) return "-"
  const date = new Date(value as string)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  })
}

export function formatRelative(value?: string): string {
  if (isZeroTime(value)) return "从未"
  const date = new Date(value as string)
  if (Number.isNaN(date.getTime())) return "-"
  const diff = Date.now() - date.getTime()
  const sec = Math.round(diff / 1000)
  if (sec < 60) return `${sec} 秒前`
  const min = Math.round(sec / 60)
  if (min < 60) return `${min} 分钟前`
  const hour = Math.round(min / 60)
  if (hour < 24) return `${hour} 小时前`
  const day = Math.round(hour / 24)
  return `${day} 天前`
}

export function formatDate(value?: string): string {
  if (isZeroTime(value)) return "-"
  const date = new Date(value as string)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  })
}

export function formatCertExpiry(cert?: CertificateInfo): string {
  if (!cert || isZeroTime(cert.not_after)) return "-"
  const date = formatDate(cert.not_after)
  if (cert.days_remaining === undefined) return date
  if (cert.days_remaining < 0) return `${date} · 已过期`
  return `${date} · 剩 ${cert.days_remaining} 天`
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

export const statusLabels: Record<HealthStatus, string> = {
  Healthy: "正常",
  Warning: "警告",
  Critical: "严重",
  Down: "宕机",
  Unknown: "未知",
}

export const monitorKindLabels: Record<string, string> = {
  tcp: "TCP 端口",
  url: "URL 探测",
  tls: "TLS 证书",
  tls_port: "TLS 证书",
}

export function isTlsMonitorKind(kind?: string): boolean {
  return kind === "tls" || kind === "tls_port" || kind === "tls_certificate"
}
