import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Check, Copy, KeyRound, Plug, Plus, RefreshCw, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import type { McpToken } from "@/lib/types"
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

type ToolEntry = {
  name: string
  description: string
}

const readTools: ToolEntry[] = [
  { name: "list_servers", description: "列出 server，支持按 environment、tags 过滤。" },
  { name: "get_server", description: "按 id 查询单个 server。" },
  { name: "get_server_health", description: "查询 server 聚合健康状态及各状态 monitor 数量。" },
  { name: "list_server_events", description: "查询 server 健康状态变更事件。" },
  { name: "list_monitors", description: "列出指定 server 下的 monitor。" },
  { name: "get_monitor", description: "按 server_id + monitor_id 查询单个 monitor。" },
  { name: "list_check_results", description: "查询 monitor 探测结果，支持 RFC3339 时间范围。" },
  { name: "get_monitor_uptime", description: "查询 monitor 的 Kuma 风格滚动可用率窗口。" },
  { name: "list_monitor_groups", description: "列出所有 monitor 分组及其 alert policy。" },
  { name: "get_monitor_group", description: "按 group_id 查询单个分组。" },
  { name: "list_monitors_by_group", description: "列出指定分组下的 monitor（跨 server）。" },
]

const writeTools: ToolEntry[] = [
  { name: "create_server", description: "创建一个被监控的 server。" },
  { name: "update_server", description: "按 id 更新 server。" },
  { name: "delete_server", description: "按 id 删除 server，同时删除其下所有 monitor。" },
  { name: "create_monitor", description: "在指定 server 下创建 monitor。" },
  { name: "update_monitor", description: "按 server_id + monitor_id 更新 monitor。" },
  { name: "delete_monitor", description: "按 server_id + monitor_id 删除 monitor。" },
  { name: "create_monitor_group", description: "创建 monitor 分组及其 alert policy。" },
  { name: "update_monitor_group", description: "按 group_id 更新分组与 alert policy。" },
  { name: "delete_monitor_group", description: "按 group_id 删除分组。" },
]

function CodeBlock({ code, language = "" }: { code: string; language?: string }) {
  const [copied, setCopied] = useState(false)

  const onCopy = async () => {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error("复制失败，请手动选择文本")
    }
  }

  return (
    <div className="group relative">
      <pre className="bg-muted/60 text-foreground overflow-x-auto rounded-md border p-3 text-xs leading-relaxed">
        <code data-language={language}>{code}</code>
      </pre>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        onClick={onCopy}
        className="absolute right-1.5 top-1.5 size-7 opacity-0 transition group-hover:opacity-100 focus-visible:opacity-100"
        aria-label="复制代码"
      >
        {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
      </Button>
    </div>
  )
}

export function McpPage() {
  const endpoint = useMemo(() => {
    if (typeof window === "undefined") return "/api/mcp"
    return `${window.location.origin}/api/mcp`
  }, [])

  const claudeConfig = `{
  "mcpServers": {
    "neo-line": {
      "type": "http",
      "url": "${endpoint}",
      "headers": {
        "Authorization": "Bearer <MCP_AUTH_TOKEN>"
      }
    }
  }
}`

  const curlExample = `curl -sN ${endpoint} \\
  -H 'Authorization: Bearer <MCP_AUTH_TOKEN>' \\
  -H 'Content-Type: application/json' \\
  -H 'Accept: application/json, text/event-stream' \\
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list"
  }'`

  return (
    <div className="animate-enter flex flex-col gap-6">
      <div>
        <div className="flex items-center gap-2">
          <Plug className="text-emerald-600 size-5 dark:text-emerald-400" />
          <h1 className="text-2xl font-semibold">MCP 接入</h1>
          <Badge variant="secondary">Streamable HTTP</Badge>
        </div>
        <p className="text-muted-foreground mt-1 text-sm">
          neo-line 内置 Model Context Protocol server，允许 AI 客户端以工具调用方式读写
          server、monitor 与 monitor group。
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>端点与鉴权</CardTitle>
          <CardDescription>
            端点挂载在 <code className="bg-muted rounded px-1">/api/mcp</code>，
            使用 streamable HTTP transport。生产环境务必配置鉴权 token。
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <span className="text-muted-foreground text-xs uppercase tracking-wide">
              Endpoint URL
            </span>
            <CodeBlock code={endpoint} />
          </div>
          <div className="flex flex-col gap-2 text-sm">
            <p>
              请求需在 <code className="bg-muted rounded px-1">Authorization: Bearer &lt;token&gt;</code>{" "}
              或 <code className="bg-muted rounded px-1">X-MCP-Token: &lt;token&gt;</code>{" "}
              头中携带 token，否则返回 <code className="bg-muted rounded px-1">401</code>。
            </p>
            <ul className="text-muted-foreground ml-4 list-disc space-y-1 text-sm">
              <li>
                推荐在下方「访问 Token」中生成并管理多个 token，持久化存储于 MongoDB，可随时吊销。
              </li>
              <li>
                仍兼容环境变量{" "}
                <code className="bg-muted rounded px-1">MCP_AUTH_TOKEN</code>{" "}
                配置的静态 token。
              </li>
              <li>
                当未配置任何 token（环境变量为空且数据库无 token）时，
                <code className="bg-muted rounded px-1">/api/mcp</code>{" "}
                不做鉴权（仅适合受信任内网或本地开发）。
              </li>
              <li>读写工具共用同一组 token，没有更细粒度的权限区分。</li>
            </ul>
          </div>
        </CardContent>
      </Card>

      <TokenManager />

      <Card>
        <CardHeader>
          <CardTitle>客户端接入</CardTitle>
          <CardDescription>
            将下列配置粘贴到 MCP 客户端中，把 <code className="bg-muted rounded px-1">&lt;MCP_AUTH_TOKEN&gt;</code>{" "}
            替换为实际 token。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="claude" className="flex flex-col gap-3">
            <TabsList>
              <TabsTrigger value="claude">Claude Desktop / Code</TabsTrigger>
              <TabsTrigger value="curl">curl</TabsTrigger>
            </TabsList>
            <TabsContent value="claude">
              <CodeBlock code={claudeConfig} language="json" />
              <p className="text-muted-foreground mt-2 text-xs">
                Claude Desktop 配置文件位于{" "}
                <code className="bg-muted rounded px-1">
                  ~/Library/Application Support/Claude/claude_desktop_config.json
                </code>{" "}
                （macOS）。Claude Code CLI 使用{" "}
                <code className="bg-muted rounded px-1">~/.claude.json</code>。
              </p>
            </TabsContent>
            <TabsContent value="curl">
              <CodeBlock code={curlExample} language="bash" />
              <p className="text-muted-foreground mt-2 text-xs">
                使用 JSON-RPC 直接调用 <code className="bg-muted rounded px-1">tools/list</code>{" "}
                可以验证连接是否正常。
              </p>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>工具列表</CardTitle>
          <CardDescription>
            工具的输入 / 输出 schema 由 Go struct 通过 <code className="bg-muted rounded px-1">jsonschema</code>{" "}
            tag 自动推导，与 REST API 行为一致。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="read" className="flex flex-col gap-3">
            <TabsList>
              <TabsTrigger value="read">
                只读 <Badge variant="secondary" className="ml-1.5">{readTools.length}</Badge>
              </TabsTrigger>
              <TabsTrigger value="write">
                写入 <Badge variant="secondary" className="ml-1.5">{writeTools.length}</Badge>
              </TabsTrigger>
            </TabsList>
            <TabsContent value="read">
              <ToolList tools={readTools} />
            </TabsContent>
            <TabsContent value="write">
              <ToolList tools={writeTools} />
              <p className="text-muted-foreground mt-3 text-xs">
                写入工具直接调用底层 store 方法，会执行 group ID 校验、默认字段填充等服务端逻辑。删除
                server 会级联删除其下 monitor。
              </p>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </div>
  )
}

function TokenManager() {
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState("")
  const [newSecret, setNewSecret] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<McpToken | undefined>()

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["mcp-tokens"],
    queryFn: () => api.listMcpTokens(),
  })
  const tokens = data?.tokens ?? []

  const createMutation = useMutation({
    mutationFn: (tokenName: string) => api.createMcpToken(tokenName),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["mcp-tokens"] })
      setNewSecret(res.secret)
      setCreateOpen(false)
      setName("")
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "创建失败")
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteMcpToken(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mcp-tokens"] })
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
            <CardTitle>访问 Token</CardTitle>
            <CardDescription>
              生成多个 token 并持久化存储于 MongoDB。Token 明文仅在创建时显示一次，请妥善保存。
            </CardDescription>
          </div>
          <Button
            size="sm"
            onClick={() => {
              setName("")
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
            <DialogTitle>生成 Token</DialogTitle>
            <DialogDescription>
              为该 token 起一个便于识别的名称，例如使用方或用途。
            </DialogDescription>
          </DialogHeader>
          <form
            id="create-mcp-token"
            className="flex flex-col gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              const trimmed = name.trim()
              if (!trimmed) {
                toast.error("请输入名称")
                return
              }
              createMutation.mutate(trimmed)
            }}
          >
            <Label htmlFor="mcp-token-name">名称</Label>
            <Input
              id="mcp-token-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：claude-desktop"
              autoFocus
            />
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button
              type="submit"
              form="create-mcp-token"
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
          {newSecret && <CodeBlock code={newSecret} />}
          <DialogFooter>
            <Button onClick={() => setNewSecret(null)}>我已保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(deleting)}
        onOpenChange={(o) => !o && setDeleting(undefined)}
        title="吊销 Token"
        description={`确定要吊销「${deleting?.name}」吗？使用该 token 的客户端将立即失去访问权限。`}
        confirmText="吊销"
        pending={deleteMutation.isPending}
        onConfirm={() => deleting && deleteMutation.mutate(deleting.id)}
      />
    </Card>
  )
}

function ToolList({ tools }: { tools: ToolEntry[] }) {
  return (
    <ul className="flex flex-col divide-y rounded-md border">
      {tools.map((tool) => (
        <li
          key={tool.name}
          className="flex flex-col gap-1 px-3 py-2 sm:flex-row sm:items-baseline sm:gap-4"
        >
          <code className="text-foreground bg-muted w-fit shrink-0 rounded px-1.5 py-0.5 text-xs font-medium">
            {tool.name}
          </code>
          <span className="text-muted-foreground text-sm">{tool.description}</span>
        </li>
      ))}
    </ul>
  )
}
