import { useEffect, useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Eye, EyeOff } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type {
  CertificateIssuer,
  CertificateIssuerCAType,
  CertificateIssuerDirectoryPreview,
} from "@/lib/types"
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

const CA_OPTIONS: { value: CertificateIssuerCAType; label: string; hint?: string }[] = [
  { value: "lets_encrypt_production", label: "Let's Encrypt 生产" },
  {
    value: "lets_encrypt_staging",
    label: "Let's Encrypt Staging",
    hint: "不受公共客户端信任，仅用于集成测试",
  },
  { value: "zerossl", label: "ZeroSSL", hint: "需要 EAB 凭据" },
  { value: "google_public_ca", label: "Google Public CA", hint: "需要 EAB 凭据" },
  { value: "custom", label: "自定义 Directory", hint: "必须使用 HTTPS 且由系统根证书信任" },
]

interface CertificateIssuerFormProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  issuer?: CertificateIssuer
}

interface FormState {
  name: string
  ca_type: CertificateIssuerCAType
  email: string
  custom_directory_url: string
  account_key_pem: string
  eab_kid: string
  eab_hmac: string
  terms_of_service_agreed: boolean
}

function toFormState(issuer?: CertificateIssuer): FormState {
  return {
    name: issuer?.name ?? "",
    ca_type: (issuer?.ca_type as CertificateIssuerCAType) ?? "lets_encrypt_production",
    email: issuer?.email ?? "",
    custom_directory_url: issuer?.ca_type === "custom" ? issuer.directory_url : "",
    account_key_pem: "",
    eab_kid: "",
    eab_hmac: "",
    terms_of_service_agreed: false,
  }
}

function SecretInput({
  value,
  onChange,
  placeholder,
  id,
  multiline,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  id?: string
  multiline?: boolean
}) {
  const [visible, setVisible] = useState(false)
  return (
    <div className="relative flex items-center">
      {multiline ? (
        <textarea
          id={id}
          className="border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring flex min-h-24 w-full rounded-md border px-3 py-2 pr-9 text-sm focus-visible:ring-2 focus-visible:outline-none"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          autoComplete="off"
        />
      ) : (
        <Input
          id={id}
          type={visible ? "text" : "password"}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder}
          className="pr-9"
          autoComplete="off"
        />
      )}
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

export function CertificateIssuerForm({
  open,
  onOpenChange,
  issuer,
}: CertificateIssuerFormProps) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FormState>(toFormState(issuer))
  const [preview, setPreview] = useState<CertificateIssuerDirectoryPreview | undefined>()
  const isEdit = Boolean(issuer)
  const isReady = issuer?.registration_status === "Ready"
  const isFailed = issuer?.registration_status === "Failed"
  const identityLocked = isEdit && isReady

  useEffect(() => {
    if (open) {
      setForm(toFormState(issuer))
      setPreview(undefined)
    }
  }, [open, issuer])

  useEffect(() => {
    if (!open || isEdit) return
    let cancelled = false
    const load = async () => {
      try {
        const { preview: p } = await api.getCertificateIssuerDirectoryPreview(
          form.ca_type,
          form.ca_type === "custom" ? form.custom_directory_url : undefined
        )
        if (!cancelled) setPreview(p)
      } catch {
        if (!cancelled) setPreview(undefined)
      }
    }
    if (form.ca_type !== "custom" || form.custom_directory_url.trim()) {
      void load()
    } else {
      setPreview(undefined)
    }
    return () => {
      cancelled = true
    }
  }, [open, isEdit, form.ca_type, form.custom_directory_url])

  const mutation = useMutation({
    mutationFn: async () => {
      if (isEdit) {
        return api.updateCertificateIssuer(issuer!.id, {
          name: form.name.trim(),
          ca_type: isFailed ? form.ca_type : undefined,
          email: isFailed ? form.email.trim() : undefined,
          custom_directory_url: isFailed && form.ca_type === "custom" ? form.custom_directory_url.trim() : undefined,
          account_key_pem: isFailed ? form.account_key_pem.trim() || undefined : undefined,
          eab_kid: isFailed ? form.eab_kid.trim() || undefined : undefined,
          eab_hmac: isFailed ? form.eab_hmac.trim() || undefined : undefined,
        })
      }
      return api.createCertificateIssuer({
        name: form.name.trim(),
        ca_type: form.ca_type,
        email: form.email.trim(),
        custom_directory_url:
          form.ca_type === "custom" ? form.custom_directory_url.trim() : undefined,
        account_key_pem: form.account_key_pem.trim() || undefined,
        eab_kid: form.eab_kid.trim() || undefined,
        eab_hmac: form.eab_hmac.trim() || undefined,
        terms_of_service_agreed: form.terms_of_service_agreed,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["certificate-issuers"] })
      toast.success(isEdit ? "Issuer 已更新" : "Issuer 已创建，正在注册 ACME 账户…")
      onOpenChange(false)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "保存失败")
    },
  })

  const selectedCA = CA_OPTIONS.find((o) => o.value === form.ca_type)
  const requiresEAB =
    (preview?.requires_eab ?? false) ||
    form.ca_type === "zerossl" ||
    form.ca_type === "google_public_ca"
  const tosURL = preview?.terms_of_service_url ?? issuer?.terms_of_service_url

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? "编辑 Issuer" : "新增 ACME Issuer"}</DialogTitle>
          <DialogDescription>
            配置 ACME 账户并在后台异步注册。只有状态为 Ready 的 Issuer 可用于证书签发。
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
            <Label htmlFor="issuer-name">显示名称</Label>
            <Input
              id="issuer-name"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              required
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="issuer-ca">CA 类型</Label>
            <select
              id="issuer-ca"
              className="border-input bg-background h-9 w-full rounded-md border px-3 text-sm"
              value={form.ca_type}
              disabled={identityLocked || (isEdit && !isFailed)}
              onChange={(e) =>
                setForm({ ...form, ca_type: e.target.value as CertificateIssuerCAType })
              }
            >
              {CA_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            {selectedCA?.hint ? (
              <p className="text-muted-foreground text-xs">{selectedCA.hint}</p>
            ) : null}
            {(preview?.staging_untrusted || issuer?.staging_untrusted) && (
              <p className="text-amber-600 text-xs dark:text-amber-400">
                此 Issuer 使用 staging / 不受公共信任的环境，签发的证书不应部署到生产。
              </p>
            )}
          </div>
          {(form.ca_type === "custom" || issuer?.ca_type === "custom") && !identityLocked && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="issuer-directory">自定义 Directory URL</Label>
              <Input
                id="issuer-directory"
                value={form.custom_directory_url}
                disabled={identityLocked}
                onChange={(e) => setForm({ ...form, custom_directory_url: e.target.value })}
                placeholder="https://acme.example.com/directory"
                required={form.ca_type === "custom" && !isEdit}
              />
            </div>
          )}
          <div className="flex flex-col gap-2">
            <Label htmlFor="issuer-email">账户邮箱</Label>
            <Input
              id="issuer-email"
              type="email"
              value={form.email}
              disabled={identityLocked}
              onChange={(e) => setForm({ ...form, email: e.target.value })}
              required={!isEdit || isFailed}
            />
          </div>
          {(!isEdit || isFailed) && requiresEAB && (
            <>
              <div className="flex flex-col gap-2">
                <Label htmlFor="issuer-eab-kid">EAB KID</Label>
                <SecretInput
                  id="issuer-eab-kid"
                  value={form.eab_kid}
                  onChange={(v) => setForm({ ...form, eab_kid: v })}
                  placeholder={isEdit ? "留空保留现有 EAB" : "External Account Binding KID"}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="issuer-eab-hmac">EAB HMAC</Label>
                <SecretInput
                  id="issuer-eab-hmac"
                  value={form.eab_hmac}
                  onChange={(v) => setForm({ ...form, eab_hmac: v })}
                  placeholder={isEdit ? "留空保留现有 EAB" : "Base64url HMAC key"}
                />
              </div>
            </>
          )}
          {(!isEdit || isFailed) && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="issuer-key">Account Key PEM（可选）</Label>
              <SecretInput
                id="issuer-key"
                multiline
                value={form.account_key_pem}
                onChange={(v) => setForm({ ...form, account_key_pem: v })}
                placeholder={isEdit ? "留空保留现有 account key" : "留空将自动生成 EC P-256 私钥"}
              />
            </div>
          )}
          {!isEdit && (
            <div className="rounded-md border p-3 text-sm">
              {preview?.directory_url && (
                <p className="text-muted-foreground mb-2 break-all text-xs">
                  Directory: {preview.directory_url}
                </p>
              )}
              {tosURL ? (
                <p className="mb-2">
                  <a
                    href={tosURL}
                    target="_blank"
                    rel="noreferrer"
                    className="text-brand underline"
                  >
                    查看 ACME Terms of Service
                  </a>
                </p>
              ) : (
                <p className="text-muted-foreground mb-2 text-xs">
                  正在加载 Directory 元数据…
                </p>
              )}
              <label className="flex items-start gap-2">
                <input
                  type="checkbox"
                  className="mt-1"
                  checked={form.terms_of_service_agreed}
                  onChange={(e) =>
                    setForm({ ...form, terms_of_service_agreed: e.target.checked })
                  }
                />
                <span>我已阅读并同意上述 ACME Terms of Service</span>
              </label>
            </div>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button
              type="submit"
              disabled={
                mutation.isPending ||
                (!isEdit && !form.terms_of_service_agreed) ||
                (!isEdit && form.ca_type === "custom" && !form.custom_directory_url.trim())
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
