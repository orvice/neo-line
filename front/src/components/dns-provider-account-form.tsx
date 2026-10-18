import { useEffect, useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Eye, EyeOff } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { DNSProviderAccount } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

interface DNSProviderAccountFormProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  account?: DNSProviderAccount
}

interface FormState {
  name: string
  propagation_timeout_seconds: number
  api_token: string
}

const DEFAULT_TIMEOUT = 120

function toFormState(account?: DNSProviderAccount): FormState {
  return {
    name: account?.name ?? "",
    propagation_timeout_seconds:
      account?.propagation_timeout_seconds ?? DEFAULT_TIMEOUT,
    api_token: "",
  }
}

function SecretInput({
  value,
  onChange,
  placeholder,
  id,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  id?: string
}) {
  const [visible, setVisible] = useState(false)
  return (
    <div className="relative flex items-center">
      <Input
        id={id}
        type={visible ? "text" : "password"}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="pr-9"
        autoComplete="off"
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

export function DNSProviderAccountForm({
  open,
  onOpenChange,
  account,
}: DNSProviderAccountFormProps) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>(toFormState(account))
  const isEdit = Boolean(account)

  useEffect(() => {
    if (open) setForm(toFormState(account))
  }, [open, account])

  const mutation = useMutation({
    mutationFn: async () => {
      const body = {
        name: form.name.trim(),
        provider: "cloudflare" as const,
        propagation_timeout_seconds: form.propagation_timeout_seconds,
        api_token: form.api_token.trim() || undefined,
      }
      return isEdit
        ? api.updateDNSProviderAccount(account!.id, body)
        : api.createDNSProviderAccount(body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["dns-provider-accounts"] })
      toast.success(isEdit ? "DNS 账户已更新" : "DNS 账户已创建")
      onOpenChange(false)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "保存失败")
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEdit ? "编辑 DNS 账户" : "新增 DNS 账户"}</DialogTitle>
          <DialogDescription>
            配置 Cloudflare API Token 用于 ACME DNS-01 验证。Token 保存前会调用
            Cloudflare 验证接口；保存后无法再次查看明文。
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
            <Label htmlFor="dns-name">名称</Label>
            <Input
              id="dns-name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="dns-provider">提供商</Label>
            <Input id="dns-provider" value="cloudflare" readOnly disabled />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="dns-timeout">DNS 传播超时（秒）</Label>
            <Input
              id="dns-timeout"
              type="number"
              min={30}
              max={900}
              value={form.propagation_timeout_seconds}
              onChange={(e) =>
                setForm({
                  ...form,
                  propagation_timeout_seconds: Number(e.target.value) || DEFAULT_TIMEOUT,
                })
              }
              required
            />
            <p className="text-muted-foreground text-xs">
              默认 120 秒，有效范围 30–900 秒。
            </p>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="dns-token">
              Cloudflare API Token{isEdit ? "（留空保留现有 Token）" : ""}
            </Label>
            <SecretInput
              id="dns-token"
              value={form.api_token}
              onChange={(v) => setForm({ ...form, api_token: v })}
              placeholder={isEdit ? "输入新 Token 以轮换…" : "Cloudflare API Token"}
            />
            <p className="text-muted-foreground text-xs">
              Token 需具备目标 Zone 的 Zone:Read 与 DNS:Edit 权限。
            </p>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button
              type="submit"
              disabled={
                mutation.isPending ||
                (!isEdit && !form.api_token.trim())
              }
            >
              {mutation.isPending ? "保存中…" : "保存"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
