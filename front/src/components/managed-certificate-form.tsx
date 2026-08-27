import { useEffect, useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { CertificateKeyType, ManagedCertificate } from "@/lib/types"
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

interface ManagedCertificateFormProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  certificate?: ManagedCertificate
  onSaved?: (cert: ManagedCertificate) => void
}

interface FormState {
  name: string
  domainsText: string
  certificateIssuerId: string
  dnsProviderAccountId: string
  keyType: CertificateKeyType
  autoRenewEnabled: boolean
  renewBeforeDays: string
  notifyGroupIds: string[]
  serverIds: string[]
}

function toFormState(cert?: ManagedCertificate): FormState {
  return {
    name: cert?.name ?? "",
    domainsText: cert?.domains?.join("\n") ?? "",
    certificateIssuerId: cert?.certificate_issuer_id ?? "",
    dnsProviderAccountId: cert?.dns_provider_account_id ?? "",
    keyType: cert?.key_type === "rsa_2048" ? "rsa_2048" : "ec_p256",
    autoRenewEnabled: cert?.auto_renew_enabled ?? true,
    renewBeforeDays:
      cert?.renew_before_days !== undefined
        ? String(cert.renew_before_days)
        : "30",
    notifyGroupIds: cert?.notify_group_ids ? [...cert.notify_group_ids] : [],
    serverIds: cert?.server_ids ? [...cert.server_ids] : [],
  }
}

function parseDomains(text: string): string[] {
  return text
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}

function isInFlight(cert?: ManagedCertificate): boolean {
  const status = cert?.latest_operation?.status
  return status === "Pending" || status === "Running"
}

export function ManagedCertificateForm({
  open,
  onOpenChange,
  certificate,
  onSaved,
}: ManagedCertificateFormProps) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>(toFormState(certificate))
  const isEdit = Boolean(certificate)
  const issueFieldsLocked = isEdit && isInFlight(certificate)

  useEffect(() => {
    if (open) setForm(toFormState(certificate))
  }, [open, certificate])

  const { data: issuerData } = useQuery({
    queryKey: ["certificate-issuers"],
    queryFn: () => api.listCertificateIssuers({ page_size: 200 }),
    enabled: open,
  })
  const { data: dnsData } = useQuery({
    queryKey: ["dns-provider-accounts"],
    queryFn: () => api.listDNSProviderAccounts({ page_size: 200 }),
    enabled: open,
  })
  const { data: notifyData } = useQuery({
    queryKey: ["notify-groups"],
    queryFn: () => api.listNotifyGroups({ page_size: 200 }),
    enabled: open,
  })
  const { data: serverData } = useQuery({
    queryKey: ["servers"],
    queryFn: () => api.listServers({ page_size: 200 }),
    enabled: open,
  })

  const readyIssuers = useMemo(
    () => (issuerData?.issuers ?? []).filter((i) => i.registration_status === "Ready"),
    [issuerData]
  )
  const notifyGroups = notifyData?.groups ?? []
  const servers = serverData?.servers ?? []
  const dnsAccounts = dnsData?.accounts ?? []

  const mutation = useMutation({
    mutationFn: async () => {
      const body = {
        name: form.name.trim(),
        domains: parseDomains(form.domainsText),
        certificate_issuer_id: form.certificateIssuerId,
        dns_provider_account_id: form.dnsProviderAccountId,
        key_type: form.keyType,
        auto_renew_enabled: form.autoRenewEnabled,
        renew_before_days: form.renewBeforeDays
          ? Number(form.renewBeforeDays)
          : 30,
        notify_group_ids: form.notifyGroupIds,
        server_ids: form.serverIds,
      }
      return isEdit
        ? api.updateManagedCertificate(certificate!.id, body)
        : api.createManagedCertificate(body)
    },
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["managed-certificates"] })
      if (certificate) {
        queryClient.invalidateQueries({
          queryKey: ["managed-certificate", certificate.id],
        })
      }
      toast.success(isEdit ? "托管证书已更新" : "托管证书已创建")
      onSaved?.(res.certificate)
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

  const toggleServer = (id: string) => {
    setForm((prev) => ({
      ...prev,
      serverIds: prev.serverIds.includes(id)
        ? prev.serverIds.filter((x) => x !== id)
        : [...prev.serverIds, id],
    }))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? "编辑托管证书" : "新增托管证书"}</DialogTitle>
          <DialogDescription>
            配置 desired config；创建后将自动提交 Pending Issue operation。域名第一行为主域名。
          </DialogDescription>
        </DialogHeader>
        {issueFieldsLocked && (
          <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-800 dark:text-amber-200">
            签发 operation 运行中：域名、Issuer、DNS 账户与密钥类型暂不可修改；仍可调整名称、Server 分配与通知组。
          </p>
        )}
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            mutation.mutate()
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="mc-name">名称</Label>
            <Input
              id="mc-name"
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="例如 prod-api"
              required
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="mc-domains">域名（每行一个，最多 100 个）</Label>
            <textarea
              id="mc-domains"
              className="border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring min-h-[88px] w-full rounded-md border px-3 py-2 text-sm focus-visible:ring-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50"
              value={form.domainsText}
              onChange={(e) =>
                setForm((p) => ({ ...p, domainsText: e.target.value }))
              }
              placeholder={"example.com\n*.example.com"}
              required
              disabled={issueFieldsLocked}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="mc-issuer">ACME Issuer（须 Ready）</Label>
            <select
              id="mc-issuer"
              className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm disabled:opacity-50"
              value={form.certificateIssuerId}
              onChange={(e) =>
                setForm((p) => ({ ...p, certificateIssuerId: e.target.value }))
              }
              required
              disabled={issueFieldsLocked}
            >
              <option value="">选择 Issuer…</option>
              {readyIssuers.map((i) => (
                <option key={i.id} value={i.id}>
                  {i.name}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="mc-dns">DNS 账户</Label>
            <select
              id="mc-dns"
              className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm disabled:opacity-50"
              value={form.dnsProviderAccountId}
              onChange={(e) =>
                setForm((p) => ({ ...p, dnsProviderAccountId: e.target.value }))
              }
              required
              disabled={issueFieldsLocked}
            >
              <option value="">选择 DNS 账户…</option>
              {dnsAccounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="mc-key">密钥类型</Label>
            <select
              id="mc-key"
              className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm disabled:opacity-50"
              value={form.keyType}
              onChange={(e) =>
                setForm((p) => ({
                  ...p,
                  keyType: e.target.value as CertificateKeyType,
                }))
              }
              disabled={issueFieldsLocked}
            >
              <option value="ec_p256">EC P-256（默认）</option>
              <option value="rsa_2048">RSA-2048</option>
            </select>
          </div>

          <div className="flex items-center justify-between rounded-md border p-3">
            <div>
              <div className="text-sm font-medium">自动续期</div>
              <div className="text-muted-foreground text-xs">默认开启</div>
            </div>
            <Switch
              checked={form.autoRenewEnabled}
              onCheckedChange={(v) =>
                setForm((p) => ({ ...p, autoRenewEnabled: v }))
              }
              disabled={issueFieldsLocked}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label htmlFor="mc-renew">续期提前天数</Label>
            <Input
              id="mc-renew"
              type="number"
              min={1}
              value={form.renewBeforeDays}
              onChange={(e) =>
                setForm((p) => ({ ...p, renewBeforeDays: e.target.value }))
              }
              disabled={issueFieldsLocked}
            />
          </div>

          <div className="flex flex-col gap-2">
            <Label>通知组（可选）</Label>
            {notifyGroups.length === 0 ? (
              <p className="text-muted-foreground text-sm">暂无通知组</p>
            ) : (
              <div className="flex max-h-32 flex-col gap-1 overflow-y-auto rounded-md border p-2">
                {notifyGroups.map((g) => (
                  <label
                    key={g.id}
                    className="flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 text-sm hover:bg-accent"
                  >
                    <input
                      type="checkbox"
                      checked={form.notifyGroupIds.includes(g.id)}
                      onChange={() => toggleNotifyGroup(g.id)}
                    />
                    {g.name}
                  </label>
                ))}
              </div>
            )}
          </div>

          <div className="flex flex-col gap-2">
            <Label>Server 分配（可选，可为空）</Label>
            {servers.length === 0 ? (
              <p className="text-muted-foreground text-sm">暂无 Server</p>
            ) : (
              <div className="flex max-h-32 flex-col gap-1 overflow-y-auto rounded-md border p-2">
                {servers.map((s) => (
                  <label
                    key={s.id}
                    className="flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 text-sm hover:bg-accent"
                  >
                    <input
                      type="checkbox"
                      checked={form.serverIds.includes(s.id)}
                      onChange={() => toggleServer(s.id)}
                    />
                    {s.name} ({s.host})
                  </label>
                ))}
              </div>
            )}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending ? "保存中…" : isEdit ? "保存" : "创建并签发"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
