import { useEffect, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { api, ApiError } from "@/lib/api"
import { useAuth } from "@/lib/auth"
import type { Settings } from "@/lib/types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

export function SettingsPage() {
  const { user } = useAuth()
  const queryClient = useQueryClient()
  const [siteName, setSiteName] = useState("")
  const [statusPageTitle, setStatusPageTitle] = useState("")

  const settingsQuery = useQuery({
    queryKey: ["settings"],
    queryFn: () => api.getSettings(),
  })
  const settings = settingsQuery.data?.settings

  useEffect(() => {
    if (settings) {
      setSiteName(settings.site_name)
      setStatusPageTitle(settings.status_page_title)
    }
  }, [settings])

  const mutation = useMutation({
    mutationFn: () => {
      const body: Partial<Settings> = {
        site_name: siteName.trim(),
        status_page_title: statusPageTitle.trim(),
      }
      return api.updateSettings(body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings"] })
      toast.success("设置已保存")
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : "保存失败")
    },
  })

  if (!user) {
    return (
      <div className="text-muted-foreground py-10 text-center">
        请先登录后再修改站点设置。
      </div>
    )
  }

  return (
    <div className="animate-enter mx-auto flex max-w-xl flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold">站点设置</h1>
        <p className="text-muted-foreground text-sm">
          配置网站名称与状态页首页标题。
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>展示设置</CardTitle>
        </CardHeader>
        <CardContent>
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault()
              mutation.mutate()
            }}
          >
            <div className="flex flex-col gap-2">
              <Label htmlFor="site_name">网站名称</Label>
              <Input
                id="site_name"
                value={siteName}
                onChange={(e) => setSiteName(e.target.value)}
                placeholder="neo-line"
              />
              <p className="text-muted-foreground text-xs">
                显示在顶部导航与浏览器标签页标题。
              </p>
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="status_page_title">首页标题</Label>
              <Input
                id="status_page_title"
                value={statusPageTitle}
                onChange={(e) => setStatusPageTitle(e.target.value)}
                placeholder="服务状态"
              />
              <p className="text-muted-foreground text-xs">
                显示在状态页（首页）顶部的大标题。
              </p>
            </div>
            <div className="flex justify-end">
              <Button
                type="submit"
                disabled={mutation.isPending || settingsQuery.isLoading}
              >
                {mutation.isPending ? "保存中…" : "保存"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
