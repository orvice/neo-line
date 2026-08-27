import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Check, Copy, KeyRound, Plus, RefreshCw, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { CertificateAccessToken } from "@/lib/types"
import { formatRelative, formatTime } from "@/lib/format"
import { ConfirmDialog } from "@/components/confirm-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

function SecretBlock({ secret }: { secret: string }) {
  const [copied, setCopied] = useState(false)

  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(secret)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error("复制失败，请手动选择文本")
    }
  }

  return (
    <div className="group relative">
      <pre className="bg-muted/60 overflow-x-auto rounded-md border p-3 text-xs leading-relaxed">
        <code>{secret}</code>
      </pre>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        onClick={onCopy}
        className="absolute right-1.5 top-1.5 size-7 opacity-0 transition group-hover:opacity-100 focus-visible:opacity-100"
        aria-label="复制 secret"
      >
        {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
      </Button>
    </div>
  )
}

export function CertificateAccessTokenPanel({ serverId }: { serverId: string }) {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [expiresAt, setExpiresAt] = useState("")
  const [newSecret, setNewSecret] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<CertificateAccessToken | undefined>()

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["certificate-access-tokens", serverId],
    queryFn: () => api.listCertificateAccessTokens(serverId),
    enabled: Boolean(serverId),
  })
  const tokens = data?.tokens ?? []

  const createMutation = useMutation({
    mutationFn: () => {
      const trimmed = name.trim()
      const exp = expiresAt.trim()
        ? new Date(expiresAt.trim())
        : undefined
      if (exp !== undefined && Number.isNaN(exp.getTime())) {
        return Promise.reject(new Error("无效的过期时间"))
      }
      return api.createCertificateAccessToken(serverId, trimmed, exp)
    },
    onSuccess: (res) => {
      queryClient.invalidateQueries({
        queryKey: ["certificate-access-tokens", serverId],
      })
      setNewSecret(res.secret)
      setCreateOpen(false)
      setName("")
      setExpiresAt("")
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "创建失败")
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (tokenId: string) =>
      api.deleteCertificateAccessToken(serverId, tokenId),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["certificate-access-tokens", serverId],
      })
      toast.success("Token 已吊销")
      setDeleting(undefined)
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "吊销失败")
    },
  })

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <CardTitle className="flex items-center gap-2 text-base">
              <KeyRound className="size-4" />
              证书访问 Token
            </CardTitle>
            <CardDescription>
              绑定本 Server 的 `nlct_` 凭据，用于后续证书 bundle 分发接口。明文仅创建时显示一次。
            </CardDescription>
          </div>
          <Button
            size="sm"
            onClick={() => {
              setName("")
              setExpiresAt("")
              setCreateOpen(true)
            }}
          >
            <Plus />
            生成 Token
          </Button>
        </div>
      </CardHeader>
      <CardContent className="px-0">
        {isLoading ? (
          <div className="text-muted-foreground p-6 text-center text-sm">
            <RefreshCw className="mx-auto size-4 animate-spin" />
          </div>
        ) : isError ? (
          <div className="text-destructive p-6 text-center text-sm">
            {error instanceof ApiError ? error.message : "加载失败"}
          </div>
        ) : tokens.length === 0 ? (
          <div className="text-muted-foreground flex flex-col items-center gap-2 p-8 text-center">
            <KeyRound className="size-8 opacity-50" />
            暂无 token，点击右上角生成。
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>前缀</TableHead>
                <TableHead>过期</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead>最近使用</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((t) => (
                <TableRow key={t.id}>
                  <TableCell className="font-medium">{t.name}</TableCell>
                  <TableCell>
                    <code className="bg-muted rounded px-1.5 py-0.5 text-xs">
                      {t.prefix}…
                    </code>
                  </TableCell>
                  <TableCell>
                    {t.expires_at ? (
                      t.expired ? (
                        <Badge variant="destructive">已过期</Badge>
                      ) : (
                        <span className="text-muted-foreground text-sm">
                          {formatTime(t.expires_at)}
                        </span>
                      )
                    ) : (
                      <span className="text-muted-foreground text-sm">不过期</span>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm">
                    {formatTime(t.created_at)}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-sm">
                    {formatRelative(t.last_used_at)}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => setDeleting(t)}
                      title="吊销"
                    >
                      <Trash2 className="text-destructive" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>生成证书访问 Token</DialogTitle>
            <DialogDescription>
              名称在同一 Server 内唯一；过期时间可选，留空表示不过期。
            </DialogDescription>
          </DialogHeader>
          <form
            id="create-cert-token"
            className="flex flex-col gap-3"
            onSubmit={(e) => {
              e.preventDefault()
              if (!name.trim()) {
                toast.error("请输入名称")
                return
              }
              createMutation.mutate()
            }}
          >
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="cert-token-name">名称</Label>
              <Input
                id="cert-token-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="例如：prod-node-1"
                autoFocus
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="cert-token-expires">过期时间（可选）</Label>
              <Input
                id="cert-token-expires"
                type="datetime-local"
                value={expiresAt}
                onChange={(e) => setExpiresAt(e.target.value)}
              />
            </div>
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button
              type="submit"
              form="create-cert-token"
              disabled={createMutation.isPending}
            >
              {createMutation.isPending ? "生成中…" : "生成"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(newSecret)}
        onOpenChange={(o) => !o && setNewSecret(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Token 已生成</DialogTitle>
            <DialogDescription>
              这是该 token 的明文，仅显示这一次。请立即复制并妥善保存，关闭后将无法再次查看。
            </DialogDescription>
          </DialogHeader>
          {newSecret && <SecretBlock secret={newSecret} />}
          <DialogFooter>
            <Button onClick={() => setNewSecret(null)}>我已保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(o) => !o && setDeleting(undefined)}
        title="吊销证书访问 Token"
        description={`确定要吊销「${deleting?.name}」吗？使用该 token 的 Server 客户端将立即失去证书下载权限。`}
        confirmText="吊销"
        pending={deleteMutation.isPending}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
    </Card>
  )
}
