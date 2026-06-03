import { useEffect, useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Eye, EyeOff, Plus, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { AlertChannel, NotifyGroup } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
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

export type ChannelType = "webhook" | "telegram" | "discord" | "mastodon"

export const CHANNEL_TYPES: { value: ChannelType; label: string }[] = [
  { value: "webhook", label: "Webhook" },
  { value: "telegram", label: "Telegram" },
  { value: "discord", label: "Discord" },
  { value: "mastodon", label: "Mastodon" },
]

// channelComplete reports whether a channel has all the fields its type needs.
export function channelComplete(c: AlertChannel): boolean {
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

interface NotifyGroupFormProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  group?: NotifyGroup
}

interface FormState {
  name: string
  description: string
  channels: AlertChannel[]
}

function toFormState(group?: NotifyGroup): FormState {
  return {
    name: group?.name ?? "",
    description: group?.description ?? "",
    channels: group?.channels?.map((c) => ({ ...c, extra: { ...c.extra } })) ?? [],
  }
}

export function NotifyGroupForm({
  open,
  onOpenChange,
  group,
}: NotifyGroupFormProps) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>(toFormState(group))
  const isEdit = Boolean(group)

  useEffect(() => {
    if (open) setForm(toFormState(group))
  }, [open, group])

  const mutation = useMutation({
    mutationFn: async () => {
      const body: Partial<NotifyGroup> = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        channels: form.channels.filter(channelComplete).map((c) => ({
          type: c.type || "webhook",
          target: c.target.trim(),
          extra: c.extra,
        })),
      }
      return isEdit
        ? api.updateNotifyGroup(group!.id, body)
        : api.createNotifyGroup(body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["notify-groups"] })
      if (group)
        queryClient.invalidateQueries({ queryKey: ["notify-group", group.id] })
      toast.success(isEdit ? "通知组已更新" : "通知组已创建")
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

  const deleteExtraKey = (index: number, key: string) => {
    setForm((prev) => {
      const channels = [...prev.channels]
      const extra = { ...channels[index].extra }
      delete extra[key]
      channels[index] = { ...channels[index], extra }
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
          <DialogTitle>{isEdit ? "编辑通知组" : "新增通知组"}</DialogTitle>
          <DialogDescription>
            通知组是一组可复用的通知通道，监控分组的告警策略通过引用通知组来派发。支持
            Webhook、Telegram、Discord、Mastodon。
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
            <Label htmlFor="ng-name">名称</Label>
            <Input
              id="ng-name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="ng-desc">描述</Label>
            <Input
              id="ng-desc"
              value={form.description}
              onChange={(e) =>
                setForm({ ...form, description: e.target.value })
              }
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
                尚未配置任何通道，请至少添加一个通知通道。
              </p>
            ) : (
              form.channels.map((channel, index) => (
                <ChannelEditor
                  key={index}
                  channel={channel}
                  onTypeChange={(type) =>
                    updateChannel(index, { type, extra: {} })
                  }
                  onTargetChange={(target) => updateChannel(index, { target })}
                  onExtraChange={(key, value) =>
                    updateExtra(index, key, value)
                  }
                  onExtraDelete={(key) => deleteExtraKey(index, key)}
                  onRemove={() => removeChannel(index)}
                />
              ))
            )}
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
  onExtraDelete: (key: string) => void
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

interface SecretInputProps {
  value: string
  onChange: (v: string) => void
  placeholder?: string
}

function SecretInput({ value, onChange, placeholder }: SecretInputProps) {
  const [visible, setVisible] = useState(false)
  return (
    <div className="relative flex items-center">
      <Input
        type={visible ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="pr-9"
      />
      <button
        type="button"
        onClick={() => setVisible((v) => !v)}
        className="text-muted-foreground hover:text-foreground absolute right-2.5 focus:outline-none"
        tabIndex={-1}
        title={visible ? "隐藏" : "显示"}
      >
        {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  )
}

function ChannelEditor({
  channel,
  onTypeChange,
  onTargetChange,
  onExtraChange,
  onExtraDelete,
  onRemove,
}: ChannelEditorProps) {
  const type = (channel.type || "webhook") as ChannelType

  // For webhook: track extra headers as key/value pairs
  const [newHeaderKey, setNewHeaderKey] = useState("")
  const [newHeaderVal, setNewHeaderVal] = useState("")

  // Reserved extra keys managed by dedicated fields
  const reservedKeys = new Set(["bot_token", "access_token", "visibility"])
  const webhookHeaders = Object.entries(channel.extra ?? {}).filter(
    ([k]) => !reservedKeys.has(k)
  )

  const addHeader = () => {
    const k = newHeaderKey.trim()
    const v = newHeaderVal.trim()
    if (!k) return
    onExtraChange(k, v)
    setNewHeaderKey("")
    setNewHeaderVal("")
  }

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
          <SecretInput
            value={channel.extra?.bot_token ?? ""}
            onChange={(v) => onExtraChange("bot_token", v)}
            placeholder="123456:ABC-DEF..."
          />
        </div>
      )}
      {type === "mastodon" && (
        <>
          <div className="flex flex-col gap-1">
            <Label className="text-xs">Access Token</Label>
            <SecretInput
              value={channel.extra?.access_token ?? ""}
              onChange={(v) => onExtraChange("access_token", v)}
              placeholder="应用访问令牌"
            />
          </div>
          <div className="flex flex-col gap-1">
            <Label className="text-xs">可见性（默认 unlisted）</Label>
            <Input
              value={channel.extra?.visibility ?? ""}
              onChange={(e) => onExtraChange("visibility", e.target.value)}
              placeholder="unlisted"
            />
          </div>
        </>
      )}

      {/* Webhook custom request headers */}
      {type === "webhook" && (
        <div className="flex flex-col gap-1">
          <Label className="text-xs">自定义请求头（可选）</Label>
          {webhookHeaders.length > 0 && (
            <div className="flex flex-col gap-1">
              {webhookHeaders.map(([k, v]) => (
                <div key={k} className="flex items-center gap-1">
                  <Input
                    className="h-7 flex-1 font-mono text-xs"
                    value={k}
                    readOnly
                  />
                  <Input
                    className="h-7 flex-1 font-mono text-xs"
                    value={v as string}
                    onChange={(e) => onExtraChange(k, e.target.value)}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-7 shrink-0"
                    onClick={() => onExtraDelete(k)}
                    title="删除"
                  >
                    <Trash2 className="text-destructive size-3" />
                  </Button>
                </div>
              ))}
            </div>
          )}
          <div className="flex items-center gap-1">
            <Input
              className="h-7 flex-1 font-mono text-xs"
              value={newHeaderKey}
              onChange={(e) => setNewHeaderKey(e.target.value)}
              placeholder="Header 名称"
            />
            <Input
              className="h-7 flex-1 font-mono text-xs"
              value={newHeaderVal}
              onChange={(e) => setNewHeaderVal(e.target.value)}
              placeholder="值"
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault()
                  addHeader()
                }
              }}
            />
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="size-7 shrink-0"
              onClick={addHeader}
              title="添加"
              disabled={!newHeaderKey.trim()}
            >
              <Plus className="size-3" />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
