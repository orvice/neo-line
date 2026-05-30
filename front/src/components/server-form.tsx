import { useEffect, useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { Server } from "@/lib/types"
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

interface ServerFormProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  server?: Server
}

interface FormState {
  name: string
  host: string
  environment: string
  region: string
  tags: string
  sortOrder: string
  enabled: boolean
}

function toFormState(server?: Server): FormState {
  return {
    name: server?.name ?? "",
    host: server?.host ?? "",
    environment: server?.environment ?? "",
    region: server?.region ?? "",
    tags: server?.tags?.join(", ") ?? "",
    sortOrder:
      server?.sort_order !== undefined ? String(server.sort_order) : "0",
    enabled: server?.enabled ?? true,
  }
}

export function ServerForm({ open, onOpenChange, server }: ServerFormProps) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>(toFormState(server))
  const isEdit = Boolean(server)

  useEffect(() => {
    if (open) setForm(toFormState(server))
  }, [open, server])

  const mutation = useMutation({
    mutationFn: async () => {
      const body: Partial<Server> = {
        name: form.name.trim(),
        host: form.host.trim(),
        environment: form.environment.trim() || undefined,
        region: form.region.trim() || undefined,
        tags: form.tags
          .split(",")
          .map((t) => t.trim())
          .filter(Boolean),
        sort_order: form.sortOrder ? Number(form.sortOrder) : 0,
        enabled: form.enabled,
      }
      return isEdit
        ? api.updateServer(server!.id, body)
        : api.createServer(body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["servers"] })
      if (server) queryClient.invalidateQueries({ queryKey: ["server", server.id] })
      toast.success(isEdit ? "服务器已更新" : "服务器已创建")
      onOpenChange(false)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "保存失败")
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{isEdit ? "编辑服务器" : "新增服务器"}</DialogTitle>
          <DialogDescription>
            配置被监控的服务器，主机字段会作为监控默认目标。
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
            <Label htmlFor="name">名称</Label>
            <Input
              id="name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="host">主机 / IP</Label>
            <Input
              id="host"
              value={form.host}
              onChange={(e) => setForm({ ...form, host: e.target.value })}
              placeholder="example.com 或 10.0.0.1"
              required
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="environment">环境</Label>
              <Input
                id="environment"
                value={form.environment}
                onChange={(e) =>
                  setForm({ ...form, environment: e.target.value })
                }
                placeholder="production"
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="region">区域</Label>
              <Input
                id="region"
                value={form.region}
                onChange={(e) => setForm({ ...form, region: e.target.value })}
                placeholder="ap-southeast-1"
              />
            </div>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="tags">标签（逗号分隔）</Label>
            <Input
              id="tags"
              value={form.tags}
              onChange={(e) => setForm({ ...form, tags: e.target.value })}
              placeholder="web, edge"
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="sort-order">排序值（越小越靠前）</Label>
            <Input
              id="sort-order"
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
          <div className="flex items-center justify-between rounded-md border p-3">
            <Label htmlFor="enabled">启用监控</Label>
            <Switch
              id="enabled"
              checked={form.enabled}
              onCheckedChange={(v) => setForm({ ...form, enabled: v })}
            />
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
