import { useEffect, useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Plus, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { AlertChannel, MonitorGroup } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
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
  enabled: boolean
  onDown: boolean
  onRecover: boolean
  onWarning: boolean
  onCritical: boolean
  minIntervalSeconds: string
  channels: AlertChannel[]
}

type ChannelType = "webhook" | "telegram" | "discord" | "mastodon"

const CHANNEL_TYPES: { value: ChannelType; label: string }[] = [
  { value: "webhook", label: "Webhook" },
  { value: "telegram", label: "Telegram" },
  { value: "discord", label: "Discord" },
  { value: "mastodon", label: "Mastodon" },
]

// channelComplete reports whether a channel has all the fields its type needs.
function channelComplete(c: AlertChannel): boolean {
  const target = c.target?.trim() ?? ""
  if (target === "") return false
  switch (c.type) {
    case "telegram":
      return Boolean(c.extra?.bot_token?.trim())
    case "mastodon":
      return Boolean(c.extra?.access_token?.trim())
    default:
      return true
  }
}

function toFormState(group?: MonitorGroup): FormState {
  const policy = group?.alert_policy
  return {
    name: group?.name ?? "",
    description: group?.description ?? "",
    enabled: policy?.enabled ?? false,
    onDown: policy?.on_down ?? true,
    onRecover: policy?.on_recover ?? true,
    onWarning: policy?.on_warning ?? false,
    onCritical: policy?.on_critical ?? true,
    minIntervalSeconds: policy?.min_interval_seconds
      ? String(policy.min_interval_seconds)
      : "",
    channels: policy?.channels?.map((c) => ({ ...c })) ?? [],
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

  const mutation = useMutation({
    mutationFn: async () => {
      const body: Partial<MonitorGroup> = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        alert_policy: {
          enabled: form.enabled,
          on_down: form.onDown,
          on_recover: form.onRecover,
          on_warning: form.onWarning,
          on_critical: form.onCritical,
          min_interval_seconds: form.minIntervalSeconds
            ? Number(form.minIntervalSeconds)
            : undefined,
          channels: form.channels.filter(channelComplete).map((c) => ({
            type: c.type || "webhook",
            target: c.target.trim(),
            extra: c.extra,
          })),
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

  const updateChannel = (index: number, patch: Partial<AlertChannel>) => {
    setForm((prev) => {
      const channels = [...prev.channels]
      channels[index] = { ...channels[index], ...patch }
      return { ...prev, channels }
    })
  }

  const updateExtra = (index: number, key: string, value: string) => {
    setForm((prev) => {
      const channels = [...prev.channels]
      channels[index] = {
        ...channels[index],
        extra: { ...channels[index].extra, [key]: value },
      }
      return { ...prev, channels }
    })
  }

  const addChannel = () => {
    setForm((prev) => ({
      ...prev,
      channels: [...prev.channels, { type: "webhook", target: "" }],
    }))
  }

  const removeChannel = (index: number) => {
    setForm((prev) => ({
      ...prev,
      channels: prev.channels.filter((_, i) => i !== index),
    }))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEdit ? "编辑分组" : "新增分组"}</DialogTitle>
          <DialogDescription>
            分组用于聚合监控项并配置共享的告警策略，支持 Webhook、Telegram、Discord、Mastodon 通道。
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
              <div className="flex items-center justify-between">
                <Label>通知通道</Label>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={addChannel}
                >
                  <Plus className="size-4" />
                  新增
                </Button>
              </div>
              {form.channels.length === 0 ? (
                <p className="text-muted-foreground text-xs">
                  尚未配置任何通道，启用告警后请至少添加一个通知通道。
                </p>
              ) : (
                form.channels.map((channel, index) => (
                  <ChannelEditor
                    key={index}
                    channel={channel}
                    onTypeChange={(type) =>
                      updateChannel(index, { type, extra: {} })
                    }
                    onTargetChange={(target) =>
                      updateChannel(index, { target })
                    }
                    onExtraChange={(key, value) =>
                      updateExtra(index, key, value)
                    }
                    onRemove={() => removeChannel(index)}
                  />
                ))
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

interface ChannelEditorProps {
  channel: AlertChannel
  onTypeChange: (type: ChannelType) => void
  onTargetChange: (target: string) => void
  onExtraChange: (key: string, value: string) => void
  onRemove: () => void
}

function targetPlaceholder(type: string): string {
  switch (type) {
    case "telegram":
      return "Chat ID，如 -1001234567890"
    case "discord":
      return "https://discord.com/api/webhooks/..."
    case "mastodon":
      return "实例地址，如 https://mastodon.social"
    default:
      return "https://hooks.example.com/..."
  }
}

function targetLabel(type: string): string {
  switch (type) {
    case "telegram":
      return "Chat ID"
    case "mastodon":
      return "实例地址"
    default:
      return "Webhook 地址"
  }
}

function ChannelEditor({
  channel,
  onTypeChange,
  onTargetChange,
  onExtraChange,
  onRemove,
}: ChannelEditorProps) {
  const type = (channel.type || "webhook") as ChannelType
  return (
    <div className="flex flex-col gap-2 rounded-md border p-3">
      <div className="flex items-center gap-2">
        <Select
          value={type}
          onValueChange={(v) => onTypeChange(v as ChannelType)}
        >
          <SelectTrigger className="w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CHANNEL_TYPES.map((t) => (
              <SelectItem key={t.value} value={t.value}>
                {t.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="ml-auto"
          onClick={onRemove}
          title="移除"
        >
          <Trash2 className="text-destructive size-4" />
        </Button>
      </div>
      <div className="flex flex-col gap-1">
        <Label className="text-xs">{targetLabel(type)}</Label>
        <Input
          value={channel.target}
          onChange={(e) => onTargetChange(e.target.value)}
          placeholder={targetPlaceholder(type)}
        />
      </div>
      {type === "telegram" && (
        <div className="flex flex-col gap-1">
          <Label className="text-xs">Bot Token</Label>
          <Input
            value={channel.extra?.bot_token ?? ""}
            onChange={(e) => onExtraChange("bot_token", e.target.value)}
            placeholder="123456:ABC-DEF..."
          />
        </div>
      )}
      {type === "mastodon" && (
        <div className="flex flex-col gap-1">
          <Label className="text-xs">Access Token</Label>
          <Input
            value={channel.extra?.access_token ?? ""}
            onChange={(e) => onExtraChange("access_token", e.target.value)}
            placeholder="应用访问令牌"
          />
        </div>
      )}
    </div>
  )
}
