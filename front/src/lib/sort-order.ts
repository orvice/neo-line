export interface Sortable {
  id: string
  sort_order: number
}

export function bySortOrder<T extends Sortable & { name: string }>(items: T[]): T[] {
  return [...items].sort(
    (a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0) || a.name.localeCompare(b.name)
  )
}

// Moves the item at `index` one slot up or down and renumbers the whole list to
// sequential sort_order values. Returns the fully reordered list plus only the
// items whose sort_order actually changed (to minimize writes). Returns null
// when the move would go out of bounds.
export function reorderByMove<T extends Sortable>(
  items: T[],
  index: number,
  dir: "up" | "down"
): { ordered: T[]; changed: T[] } | null {
  const target = dir === "up" ? index - 1 : index + 1
  if (target < 0 || target >= items.length) return null

  const swapped = items.slice()
  ;[swapped[index], swapped[target]] = [swapped[target], swapped[index]]

  const changed: T[] = []
  const ordered = swapped.map((item, i) => {
    if ((item.sort_order ?? 0) !== i) {
      const updated = { ...item, sort_order: i }
      changed.push(updated)
      return updated
    }
    return item
  })

  return { ordered, changed }
}
