import { Skeleton } from "@/components/ui/skeleton"

// TableSkeleton mirrors the shape of a loaded data table so the layout does
// not shift when real rows arrive (CLS guard).
export function TableSkeleton({
  rows = 5,
  columns = 5,
}: {
  rows?: number
  columns?: number
}) {
  return (
    <div className="divide-y">
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="flex items-center gap-4 px-4 py-3.5">
          {Array.from({ length: columns }).map((_, c) => (
            <Skeleton
              key={c}
              className="h-4"
              style={{ width: c === 0 ? "4rem" : c === 1 ? "30%" : "16%" }}
            />
          ))}
        </div>
      ))}
    </div>
  )
}
