import { Link, Outlet, useLocation } from "react-router-dom"
import { Activity, LogOut } from "lucide-react"

import { useAuth } from "@/lib/auth"
import { Button } from "@/components/ui/button"

export function Layout() {
  const { user, logout } = useAuth()
  const location = useLocation()

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-40 border-b bg-background/80 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4">
          <Link to="/" className="flex items-center gap-2 font-semibold">
            <Activity className="size-5 text-emerald-400" />
            <span>neo-line</span>
            <span className="text-muted-foreground text-sm font-normal">
              监控面板
            </span>
          </Link>
          <div className="flex items-center gap-3">
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
