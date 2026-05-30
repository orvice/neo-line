import { useEffect, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "react-router-dom"
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

export function MonitorGroupForm({
  open,
  onOpenChange,
  group,
}: MonitorGroupFormProps) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>(toFormState(group))
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

  return (
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

            <div className="grid grid-cols-2 gap-3">
              <label className="flex items-center justify-between rounded-md border p-2 text-sm">
                <span>Down</span>
                <Switch
                  checked={form.onDown}
                  onCheckedChange={(v) => setForm({ ...form, onDown: v })}
                />
              </label>
              <label className="flex items-center justify-between rounded-md border p-2 text-sm">
                <span>恢复</span>
                <Switch
                  checked={form.onRecover}
                  onCheckedChange={(v) => setForm({ ...form, onRecover: v })}
                />
              </label>
              <label className="flex items-center justify-between rounded-md border p-2 text-sm">
                <span>Critical</span>
                <Switch
                  checked={form.onCritical}
                  onCheckedChange={(v) => setForm({ ...form, onCritical: v })}
                />
              </label>
              <label className="flex items-center justify-between rounded-md border p-2 text-sm">
                <span>Warning</span>
                <Switch
                  checked={form.onWarning}
                  onCheckedChange={(v) => setForm({ ...form, onWarning: v })}
                />
              </label>
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="g-min">节流（秒，留空不节流）</Label>
              <Input
                id="g-min"
                type="number"
                value={form.minIntervalSeconds}
                onChange={(e) =>
                  setForm({ ...form, minIntervalSeconds: e.target.value })
                }
                placeholder="300"
              />
            </div>

            <div className="flex flex-col gap-2">
              <Label>通知组</Label>
              {notifyGroups.length === 0 ? (
                <p className="text-muted-foreground text-xs">
                  尚无通知组，请先到{" "}
                  <Link to="/notify-groups" className="underline">
                    通知组
                  </Link>{" "}
                  页面创建。
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
  )
}
