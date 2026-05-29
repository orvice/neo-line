import { cn } from "@/lib/utils"
import { formatTime, statusLabels } from "@/lib/format"
import type { Heartbeat, HealthStatus } from "@/lib/types"

const barStyles: Record<HealthStatus, string> = {
  Healthy: "bg-emerald-500",
  Warning: "bg-amber-500",
  Critical: "bg-orange-500",
  Down: "bg-red-500",
  Unknown: "bg-zinc-600",
}

function normalize(status: string): HealthStatus {
  if (status in barStyles) return status as HealthStatus
  return "Unknown"
}

// HeartbeatBar renders the most recent checks as a Kuma-style row of bars,
// oldest on the left. Beats are expected in chronological order.
export function HeartbeatBar({
  beats,
  max = 50,
}: {
  beats: Heartbeat[]
  max?: number
}) {
  const recent = beats.slice(-max)

  if (recent.length === 0) {
    return (
      <div className="text-muted-foreground py-2 text-xs">暂无心跳数据</div>
    )
  }

  return (
    <div className="flex items-end gap-[3px]">
      {recent.map((beat, i) => {
        const s = normalize(beat.status)
        return (
          <div
            key={`${beat.started_at}-${i}`}
            className={cn(
              "h-7 w-[5px] shrink-0 rounded-sm transition-opacity hover:opacity-70",
              barStyles[s]
            )}
            title={`${statusLabels[s]} · ${formatTime(beat.started_at)} · ${beat.duration_ms} ms`}
          />
        )
      })}
    </div>
  )
}
