import { useEffect, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { MonitorGroup } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { NotifyGroupForm } from "@/components/notify-group-form"
import { Plus } from "lucide-react"

interface MonitorGroupFormProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  group?: MonitorGroup
}

interface FormState {
  name: string
  description: string
  sortOrder: string
  enabled: boolean
  onDown: boolean
  onRecover: boolean
  onWarning: boolean
  onCritical: boolean
  minIntervalSeconds: string
  notifyGroupIds: string[]
}

function toFormState(group?: MonitorGroup): FormState {
  const policy = group?.alert_policy
  return {
    name: group?.name ?? "",
    description: group?.description ?? "",
    sortOrder:
      group?.sort_order !== undefined ? String(group.sort_order) : "0",
    enabled: policy?.enabled ?? false,
    onDown: policy?.on_down ?? true,
    onRecover: policy?.on_recover ?? true,
    onWarning: policy?.on_warning ?? false,
    onCritical: policy?.on_critical ?? true,
    minIntervalSeconds: policy?.min_interval_seconds
      ? String(policy.min_interval_seconds)
      : "",
    notifyGroupIds: policy?.notify_group_ids ? [...policy.notify_group_ids] : [],
  }
}

function formatSeconds(raw: string): string {
  const n = parseInt(raw, 10)
  if (!raw || isNaN(n) || n <= 0) return ""
  if (n < 60) return `= ${n} 秒`
  if (n < 3600) {
    const m = Math.floor(n / 60)
    const s = n % 60
    return s === 0 ? `= ${m} 分钟` : `= ${m} 分 ${s} 秒`
  }
  const h = Math.floor(n / 3600)
  const rem = n % 3600
  const m = Math.floor(rem / 60)
  return m === 0 ? `= ${h} 小时` : `= ${h} 小时 ${m} 分钟`
}

interface TriggerItemProps {
  label: string
  colorClass: string
  checked: boolean
  onCheckedChange: (v: boolean) => void
  disabled?: boolean
}

function TriggerItem({ label, colorClass, checked, onCheckedChange, disabled }: TriggerItemProps) {
  return (
    <label className={`flex items-center justify-between rounded-md border p-2 text-sm ${disabled ? "pointer-events-none opacity-50" : ""}`}>
      <span className={`font-medium ${colorClass}`}>{label}</span>
      <Switch
        checked={checked}
        onCheckedChange={onCheckedChange}
        disabled={disabled}
      />
    </label>
  )
}

export function MonitorGroupForm({
  open,
  onOpenChange,
  group,
}: MonitorGroupFormProps) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>(toFormState(group))
  const [quickCreateOpen, setQuickCreateOpen] = useState(false)
  const isEdit = Boolean(group)

  useEffect(() => {
    if (open) setForm(toFormState(group))
  }, [open, group])

  const { data: notifyData } = useQuery({
    queryKey: ["notify-groups"],
    queryFn: () => api.listNotifyGroups({ page_size: 200 }),
    enabled: open,
  })
  const notifyGroups = notifyData?.groups ?? []

  const mutation = useMutation({
    mutationFn: async () => {
      const body: Partial<MonitorGroup> = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        sort_order: form.sortOrder ? Number(form.sortOrder) : 0,
        alert_policy: {
          enabled: form.enabled,
          on_down: form.onDown,
          on_recover: form.onRecover,
          on_warning: form.onWarning,
          on_critical: form.onCritical,
          min_interval_seconds: form.minIntervalSeconds
            ? Number(form.minIntervalSeconds)
            : undefined,
          notify_group_ids: form.notifyGroupIds,
        },
      }
      return isEdit
        ? api.updateMonitorGroup(group!.id, body)
        : api.createMonitorGroup(body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["monitor-groups"] })
      if (group)
        queryClient.invalidateQueries({ queryKey: ["monitor-group", group.id] })
      toast.success(isEdit ? "分组已更新" : "分组已创建")
      onOpenChange(false)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "保存失败")
    },
  })

  const toggleNotifyGroup = (id: string) => {
    setForm((prev) => ({
      ...prev,
      notifyGroupIds: prev.notifyGroupIds.includes(id)
        ? prev.notifyGroupIds.filter((x) => x !== id)
        : [...prev.notifyGroupIds, id],
    }))
  }

  const intervalHint = formatSeconds(form.minIntervalSeconds)

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>{isEdit ? "编辑分组" : "新增分组"}</DialogTitle>
            <DialogDescription>
              分组用于聚合监控项并配置共享的告警策略，告警通过引用通知组派发。
            </DialogDescription>
          </DialogHeader>
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault()
              mutation.mutate()
            }}
          >
            <div className="flex flex-col gap-2">
              <Label htmlFor="g-name">名称</Label>
              <Input
                id="g-name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                required
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="g-desc">描述</Label>
              <Input
                id="g-desc"
                value={form.description}
                onChange={(e) =>
                  setForm({ ...form, description: e.target.value })
                }
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="g-sort">排序值（越小越靠前）</Label>
              <Input
                id="g-sort"
                type="number"
                min="0"
                step="1"
                value={form.sortOrder}
                onChange={(e) =>
                  setForm({ ...form, sortOrder: e.target.value })
                }
                placeholder="0"
              />
            </div>

            <div className="flex flex-col gap-4 rounded-md border p-3">
              <div className="flex items-center justify-between">
                <Label htmlFor="g-alert-enabled">启用告警</Label>
                <Switch
                  id="g-alert-enabled"
                  checked={form.enabled}
                  onCheckedChange={(v) => setForm({ ...form, enabled: v })}
                />
              </div>

              {/* Sub-section: only interactive when enabled */}
              <div className={`flex flex-col gap-3 transition-opacity ${form.enabled ? "" : "pointer-events-none opacity-40"}`}>
                <div className="grid grid-cols-2 gap-2">
                  <TriggerItem
                    label="Down"
                    colorClass="text-red-600 dark:text-red-400"
                    checked={form.onDown}
                    onCheckedChange={(v) => setForm({ ...form, onDown: v })}
                    disabled={!form.enabled}
                  />
                  <TriggerItem
                    label="Critical"
                    colorClass="text-orange-500 dark:text-orange-400"
                    checked={form.onCritical}
                    onCheckedChange={(v) => setForm({ ...form, onCritical: v })}
                    disabled={!form.enabled}
                  />
                  <TriggerItem
                    label="Warning"
                    colorClass="text-yellow-600 dark:text-yellow-400"
                    checked={form.onWarning}
                    onCheckedChange={(v) => setForm({ ...form, onWarning: v })}
                    disabled={!form.enabled}
                  />
                  <TriggerItem
                    label="恢复"
                    colorClass="text-emerald-600 dark:text-emerald-400"
                    checked={form.onRecover}
                    onCheckedChange={(v) => setForm({ ...form, onRecover: v })}
                    disabled={!form.enabled}
                  />
                </div>

                <div className="flex flex-col gap-1">
                  <Label htmlFor="g-min" className="text-sm">节流间隔（留空不节流）</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      id="g-min"
                      type="number"
                      min="0"
                      value={form.minIntervalSeconds}
                      onChange={(e) =>
                        setForm({ ...form, minIntervalSeconds: e.target.value })
                      }
                      placeholder="300"
                      disabled={!form.enabled}
                      className="flex-1"
                    />
                    <span className="text-muted-foreground min-w-[4rem] text-xs">秒</span>
                    {intervalHint && (
                      <span className="text-muted-foreground whitespace-nowrap text-xs">{intervalHint}</span>
                    )}
                  </div>
                </div>

                <div className="flex flex-col gap-2">
                  <div className="flex items-center justify-between">
                    <Label className="text-sm">通知组</Label>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="h-7 gap-1 text-xs"
                      onClick={() => setQuickCreateOpen(true)}
                      disabled={!form.enabled}
                    >
                      <Plus className="size-3" />
                      新建通知组
                    </Button>
                  </div>
                  {notifyGroups.length === 0 ? (
                    <p className="text-muted-foreground text-xs">
                      尚无通知组，点击上方「新建通知组」按钮创建。
                    </p>
                  ) : (
                    <div className="flex flex-col gap-1">
                      {notifyGroups.map((ng) => (
                        <label
                          key={ng.id}
                          className="flex items-center gap-2 rounded-md border p-2 text-sm"
                        >
                          <input
                            type="checkbox"
                            className="size-4"
                            checked={form.notifyGroupIds.includes(ng.id)}
                            onChange={() => toggleNotifyGroup(ng.id)}
                            disabled={!form.enabled}
                          />
                          <span className="font-medium">{ng.name}</span>
                          <span className="text-muted-foreground">
                            {ng.channels?.length ?? 0} 个通道
                          </span>
                        </label>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                取消
              </Button>
              <Button type="submit" disabled={mutation.isPending}>
                {mutation.isPending ? "保存中…" : "保存"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Quick-create notify group without navigating away */}
      <NotifyGroupForm
        open={quickCreateOpen}
        onOpenChange={(o) => {
          setQuickCreateOpen(o)
          if (!o) {
            queryClient.invalidateQueries({ queryKey: ["notify-groups"] })
          }
        }}
      />
    </>
  )
}
