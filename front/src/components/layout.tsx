import { useEffect } from "react"
import { Link, NavLink, Outlet, useLocation } from "react-router-dom"
import {
  Activity,
  BellRing,
  FileSearch,
  FolderTree,
  LogOut,
  Monitor,
  Moon,
  Plug,
  Server,
  Settings as SettingsIcon,
  Sun,
} from "lucide-react"

import { useAuth } from "@/lib/auth"
import { useTheme } from "@/lib/theme"
import { useSettings } from "@/lib/settings"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

const themeOrder = ["light", "dark", "system"] as const
const themeIcon = { light: Sun, dark: Moon, system: Monitor }
const themeLabel = { light: "浅色", dark: "深色", system: "跟随系统" }

function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const Icon = themeIcon[theme]
  const next = themeOrder[(themeOrder.indexOf(theme) + 1) % themeOrder.length]
  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={() => setTheme(next)}
      title={`主题：${themeLabel[theme]}（点击切换）`}
      aria-label={`切换主题，当前${themeLabel[theme]}`}
    >
      <Icon className="size-4" />
    </Button>
  )
}

export function Layout() {
  const { user, logout } = useAuth()
  const location = useLocation()
  const settings = useSettings()
  const isStatusPage = location.pathname === "/"

  useEffect(() => {
    document.title = settings.site_name
  }, [settings.site_name])

  const navClass = ({ isActive }: { isActive: boolean }) =>
    cn(
      "flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition",
      isActive
        ? "bg-card text-foreground shadow-xs ring ring-hairline"
        : "text-muted-foreground hover:bg-accent hover:text-foreground"
    )

  return (
    <div className="min-h-[100dvh] bg-background">
      <header className="sticky top-0 z-40 border-b bg-surface-elevated/85 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4">
          <Link to="/" className="flex items-center gap-2 font-semibold">
            <Activity className="size-5 text-brand" />
            <span>{settings.site_name}</span>
            <span className="text-sm font-normal text-muted-foreground">
              监控面板
            </span>
          </Link>
          <nav className="hidden items-center gap-1 sm:flex">
            <NavLink
              to="/"
              end
              className={navClass}
            >
              <Activity className="size-4" />
              状态
            </NavLink>
            {user ? (
              <>
                <NavLink
                  to="/servers"
                  className={navClass}
                >
                  <Server className="size-4" />
                  服务器
                </NavLink>
                <NavLink
                  to="/monitor-groups"
                  className={navClass}
                >
                  <FolderTree className="size-4" />
                  分组
                </NavLink>
                <NavLink
                  to="/notify-groups"
                  className={navClass}
                >
                  <BellRing className="size-4" />
                  通知组
                </NavLink>
                <NavLink
                  to="/mcp"
                  className={navClass}
                >
                  <Plug className="size-4" />
                  MCP
                </NavLink>
                <NavLink
                  to="/audit-logs"
                  className={navClass}
                >
                  <FileSearch className="size-4" />
                  审计
                </NavLink>
                <NavLink
                  to="/settings"
                  className={navClass}
                >
                  <SettingsIcon className="size-4" />
                  设置
                </NavLink>
              </>
            ) : null}
          </nav>
          <div className="flex items-center gap-2">
            <ThemeToggle />
            {user ? (
              <>
                <span className="hidden text-sm text-muted-foreground sm:inline">
                  {user.email}
                </span>
                <Button variant="ghost" size="sm" onClick={() => logout()}>
                  <LogOut className="size-4" />
                  退出
                </Button>
              </>
            ) : (
              <Button asChild variant="outline" size="sm">
                <Link
                  to="/login"
                  state={{ from: location.pathname }}
                >
                  登录
                </Link>
              </Button>
            )}
          </div>
        </div>
      </header>
      <main
        className={cn(
          isStatusPage ? "mx-0 max-w-none px-0 py-0" : "mx-auto max-w-6xl px-4 py-6"
        )}
      >
        <Outlet />
      </main>
    </div>
  )
}
