import { useEffect } from "react"
import { Link, NavLink, Outlet, useLocation } from "react-router-dom"
import {
  Activity,
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

  useEffect(() => {
    document.title = settings.site_name
  }, [settings.site_name])

  return (
    <div className="min-h-[100dvh] bg-background">
      <header className="sticky top-0 z-40 border-b bg-background/80 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4">
          <Link to="/" className="flex items-center gap-2 font-semibold">
            <Activity className="size-5 text-emerald-600 dark:text-emerald-400" />
            <span>{settings.site_name}</span>
            <span className="text-muted-foreground text-sm font-normal">
              监控面板
            </span>
          </Link>
          <nav className="hidden items-center gap-1 sm:flex">
            <NavLink
              to="/"
              end
              className={({ isActive }) =>
                "flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition " +
                (isActive
                  ? "bg-accent text-accent-foreground"
                  : "text-muted-foreground hover:bg-accent")
              }
            >
              <Activity className="size-4" />
              状态
            </NavLink>
            {user ? (
              <>
                <NavLink
                  to="/servers"
                  className={({ isActive }) =>
                    "flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition " +
                    (isActive
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent")
                  }
                >
                  <Server className="size-4" />
                  服务器
                </NavLink>
                <NavLink
                  to="/monitor-groups"
                  className={({ isActive }) =>
                    "flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition " +
                    (isActive
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent")
                  }
                >
                  <FolderTree className="size-4" />
                  分组
                </NavLink>
                <NavLink
                  to="/mcp"
                  className={({ isActive }) =>
                    "flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition " +
                    (isActive
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent")
                  }
                >
                  <Plug className="size-4" />
                  MCP
                </NavLink>
                <NavLink
                  to="/settings"
                  className={({ isActive }) =>
                    "flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition " +
                    (isActive
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent")
                  }
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
                <span className="text-muted-foreground hidden text-sm sm:inline">
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
      <main className="mx-auto max-w-6xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}
