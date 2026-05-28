import { useEffect, useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { Monitor, MonitorKind } from "@/lib/types"
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

interface MonitorFormProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  serverId: string
  monitor?: Monitor
}

interface FormState {
  name: string
  kind: MonitorKind
  enabled: boolean
  host: string
  port: string
  url: string
  method: string
  expectedStatus: string
  tlsVerify: boolean
  sniName: string
  warningDays: string
  criticalDays: string
  intervalSeconds: string
  timeoutSeconds: string
  retries: string
}

function toFormState(monitor?: Monitor): FormState {
  return {
    name: monitor?.name ?? "",
    kind: monitor?.kind ?? "tcp",
    enabled: monitor?.enabled ?? true,
    host: monitor?.host ?? "",
    port: monitor?.port ? String(monitor.port) : "",
    url: monitor?.url ?? "",
    method: monitor?.method ?? "GET",
    expectedStatus: monitor?.expected_status_codes?.join(", ") ?? "",
    tlsVerify: monitor?.tls_verify ?? true,
    sniName: monitor?.sni_name ?? "",
    warningDays: monitor?.warning_days ? String(monitor.warning_days) : "",
    criticalDays: monitor?.critical_days ? String(monitor.critical_days) : "",
    intervalSeconds: monitor?.interval_seconds ? String(monitor.interval_seconds) : "60",
    timeoutSeconds: monitor?.timeout_seconds ? String(monitor.timeout_seconds) : "5",
    retries: monitor?.retries !== undefined ? String(monitor.retries) : "3",
  }
}

function numberOrUndefined(value: string): number | undefined {
  const n = Number(value)
  return value.trim() === "" || Number.isNaN(n) ? undefined : n
}

export function MonitorForm({
  open,
  onOpenChange,
  serverId,
  monitor,
}: MonitorFormProps) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>(toFormState(monitor))
  const isEdit = Boolean(monitor)

  useEffect(() => {
    if (open) setForm(toFormState(monitor))
  }, [open, monitor])

  const mutation = useMutation({
    mutationFn: async () => {
      const body: Partial<Monitor> = {
        name: form.name.trim(),
        kind: form.kind,
        enabled: form.enabled,
        interval_seconds: numberOrUndefined(form.intervalSeconds),
        timeout_seconds: numberOrUndefined(form.timeoutSeconds),
        retries: numberOrUndefined(form.retries),
      }
      if (form.kind === "tcp" || form.kind === "tls_port") {
        body.host = form.host.trim() || undefined
        body.port = numberOrUndefined(form.port)
      }
      if (form.kind === "url") {
        body.url = form.url.trim()
        body.method = form.method.trim() || "GET"
        body.expected_status_codes = form.expectedStatus
          .split(",")
          .map((s) => Number(s.trim()))
          .filter((n) => !Number.isNaN(n) && n > 0)
      }
      if (form.kind === "url" || form.kind === "tls_port") {
        body.tls_verify = form.tlsVerify
        body.sni_name = form.sniName.trim() || undefined
      }
      if (form.kind === "tls_port") {
        body.warning_days = numberOrUndefined(form.warningDays)
        body.critical_days = numberOrUndefined(form.criticalDays)
      }
      return isEdit
        ? api.updateMonitor(serverId, monitor!.id, body)
        : api.createMonitor(serverId, body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["monitors", serverId] })
      queryClient.invalidateQueries({ queryKey: ["server-health", serverId] })
      if (monitor)
        queryClient.invalidateQueries({ queryKey: ["monitor", serverId, monitor.id] })
      toast.success(isEdit ? "监控项已更新" : "监控项已创建")
      onOpenChange(false)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "保存失败")
    },
  })

  const showHostPort = form.kind === "tcp" || form.kind === "tls_port"
  const showTls = form.kind === "url" || form.kind === "tls_port"

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEdit ? "编辑监控项" : "新增监控项"}</DialogTitle>
          <DialogDescription>
            配置探测类型与参数，留空字段将使用服务端默认值。
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
            <Label htmlFor="m-name">名称</Label>
            <Input
              id="m-name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              required
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label>类型</Label>
            <Select
              value={form.kind}
              onValueChange={(v) => setForm({ ...form, kind: v as MonitorKind })}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="tcp">TCP 端口</SelectItem>
                <SelectItem value="url">URL 探测</SelectItem>
                <SelectItem value="tls_port">TLS 证书</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {showHostPort && (
            <div className="grid grid-cols-3 gap-4">
              <div className="col-span-2 flex flex-col gap-2">
                <Label htmlFor="m-host">主机（留空用服务器主机）</Label>
                <Input
                  id="m-host"
                  value={form.host}
                  onChange={(e) => setForm({ ...form, host: e.target.value })}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="m-port">端口</Label>
                <Input
                  id="m-port"
                  type="number"
                  value={form.port}
                  onChange={(e) => setForm({ ...form, port: e.target.value })}
                  placeholder={form.kind === "tls_port" ? "443" : ""}
                />
              </div>
            </div>
          )}

          {form.kind === "url" && (
            <>
              <div className="flex flex-col gap-2">
                <Label htmlFor="m-url">URL</Label>
                <Input
                  id="m-url"
                  value={form.url}
                  onChange={(e) => setForm({ ...form, url: e.target.value })}
                  placeholder="https://example.com/health"
                  required
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="m-method">方法</Label>
                  <Input
                    id="m-method"
                    value={form.method}
                    onChange={(e) =>
                      setForm({ ...form, method: e.target.value })
                    }
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="m-status">期望状态码</Label>
                  <Input
                    id="m-status"
                    value={form.expectedStatus}
                    onChange={(e) =>
                      setForm({ ...form, expectedStatus: e.target.value })
                    }
                    placeholder="200, 204"
                  />
                </div>
              </div>
            </>
          )}

          {showTls && (
            <div className="flex flex-col gap-4 rounded-md border p-3">
              <div className="flex items-center justify-between">
                <Label htmlFor="m-tls">校验 TLS 证书</Label>
                <Switch
                  id="m-tls"
                  checked={form.tlsVerify}
                  onCheckedChange={(v) => setForm({ ...form, tlsVerify: v })}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="m-sni">自定义 SNI 名称</Label>
                <Input
                  id="m-sni"
                  value={form.sniName}
                  onChange={(e) =>
                    setForm({ ...form, sniName: e.target.value })
                  }
                  placeholder="example.com"
                />
              </div>
              {form.kind === "tls_port" && (
                <div className="grid grid-cols-2 gap-4">
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="m-warn">警告天数</Label>
                    <Input
                      id="m-warn"
                      type="number"
                      value={form.warningDays}
                      onChange={(e) =>
                        setForm({ ...form, warningDays: e.target.value })
                      }
                      placeholder="30"
                    />
                  </div>
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="m-crit">严重天数</Label>
                    <Input
                      id="m-crit"
                      type="number"
                      value={form.criticalDays}
                      onChange={(e) =>
                        setForm({ ...form, criticalDays: e.target.value })
                      }
                      placeholder="7"
                    />
                  </div>
                </div>
              )}
            </div>
          )}

          <div className="grid grid-cols-3 gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="m-interval">间隔（秒）</Label>
              <Input
                id="m-interval"
                type="number"
                value={form.intervalSeconds}
                onChange={(e) =>
                  setForm({ ...form, intervalSeconds: e.target.value })
                }
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="m-timeout">超时（秒）</Label>
              <Input
                id="m-timeout"
                type="number"
                value={form.timeoutSeconds}
                onChange={(e) =>
                  setForm({ ...form, timeoutSeconds: e.target.value })
                }
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="m-retries">重试次数</Label>
              <Input
                id="m-retries"
                type="number"
                value={form.retries}
                onChange={(e) => setForm({ ...form, retries: e.target.value })}
              />
            </div>
          </div>

          <div className="flex items-center justify-between rounded-md border p-3">
            <Label htmlFor="m-enabled">启用</Label>
            <Switch
              id="m-enabled"
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
