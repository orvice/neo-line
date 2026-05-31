import { ChevronDown, ChevronUp } from "lucide-react"

import { Button } from "@/components/ui/button"

interface ReorderControlsProps {
  order: number
  canUp: boolean
  canDown: boolean
  disabled?: boolean
  onUp: () => void
  onDown: () => void
}

export function ReorderControls({
  order,
  canUp,
  canDown,
  disabled,
  onUp,
  onDown,
}: ReorderControlsProps) {
  return (
    <div className="flex items-center gap-1.5">
      <div className="flex flex-col">
        <Button
          variant="ghost"
          size="icon"
          className="size-5"
          disabled={disabled || !canUp}
          onClick={onUp}
          title="上移"
        >
          <ChevronUp className="size-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="size-5"
          disabled={disabled || !canDown}
          onClick={onDown}
          title="下移"
        >
          <ChevronDown className="size-3.5" />
        </Button>
      </div>
      <span className="text-muted-foreground tabular-nums">{order}</span>
    </div>
  )
}
