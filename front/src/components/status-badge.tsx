import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { statusLabels } from "@/lib/format"
import type { HealthStatus } from "@/lib/types"

const styles: Record<HealthStatus, string> = {
  Healthy:
    "bg-emerald-500/10 text-emerald-700 border-emerald-600/30 dark:bg-emerald-500/15 dark:text-emerald-400 dark:border-emerald-500/30",
  Warning:
    "bg-amber-500/10 text-amber-700 border-amber-600/30 dark:bg-amber-500/15 dark:text-amber-400 dark:border-amber-500/30",
  Critical:
    "bg-orange-500/10 text-orange-700 border-orange-600/30 dark:bg-orange-500/15 dark:text-orange-400 dark:border-orange-500/30",
  Down: "bg-red-500/10 text-red-700 border-red-600/30 dark:bg-red-500/15 dark:text-red-400 dark:border-red-500/30",
  Unknown:
    "bg-zinc-500/10 text-zinc-600 border-zinc-500/30 dark:bg-zinc-500/15 dark:text-zinc-400",
}

const dotStyles: Record<HealthStatus, string> = {
  Healthy: "bg-emerald-500 dark:bg-emerald-400",
  Warning: "bg-amber-500 dark:bg-amber-400",
  Critical: "bg-orange-500 dark:bg-orange-400",
  Down: "bg-red-500 dark:bg-red-400",
  Unknown: "bg-zinc-500 dark:bg-zinc-400",
}

function normalize(status: string): HealthStatus {
  if (status in styles) return status as HealthStatus
  return "Unknown"
}

export function StatusBadge({ status }: { status: string }) {
  const s = normalize(status)
  return (
    <Badge variant="outline" className={cn("gap-1.5", styles[s])}>
      <span className={cn("size-1.5 rounded-full", dotStyles[s])} />
      {statusLabels[s]}
    </Badge>
  )
}

export function StatusDot({ status }: { status: string }) {
  const s = normalize(status)
  return <span className={cn("inline-block size-2.5 rounded-full", dotStyles[s])} />
}
