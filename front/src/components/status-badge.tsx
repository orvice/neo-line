import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { statusLabels } from "@/lib/format"
import type { HealthStatus } from "@/lib/types"

const styles: Record<HealthStatus, string> = {
  Healthy: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30",
  Warning: "bg-amber-500/15 text-amber-400 border-amber-500/30",
  Critical: "bg-orange-500/15 text-orange-400 border-orange-500/30",
  Down: "bg-red-500/15 text-red-400 border-red-500/30",
  Unknown: "bg-zinc-500/15 text-zinc-400 border-zinc-500/30",
}

const dotStyles: Record<HealthStatus, string> = {
  Healthy: "bg-emerald-400",
  Warning: "bg-amber-400",
  Critical: "bg-orange-400",
  Down: "bg-red-400",
  Unknown: "bg-zinc-400",
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
