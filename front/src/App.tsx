import { Navigate, Route, Routes, useLocation } from "react-router-dom"
import type { ReactElement } from "react"

import { useAuth } from "./lib/auth"
import { Layout } from "./components/layout"
import { LoginPage } from "./pages/login"
import { StatusPage } from "./pages/status"
import { ServersPage } from "./pages/servers"
import { ServerDetailPage } from "./pages/server-detail"
import { MonitorDetailPage } from "./pages/monitor-detail"
import { MonitorGroupsPage } from "./pages/monitor-groups"
import { MonitorGroupDetailPage } from "./pages/monitor-group-detail"
import { SettingsPage } from "./pages/settings"
import { McpPage } from "./pages/mcp"

function LoadingScreen() {
  return (
    <div className="flex min-h-[100dvh] items-center justify-center text-muted-foreground">
      加载中…
    </div>
  )
}

function RequireAuth({ children }: { children: ReactElement }) {
  const { user } = useAuth()
  const location = useLocation()
  if (!user) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  return children
}

export function App() {
  const { loading } = useAuth()
  if (loading) return <LoadingScreen />

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<Layout />}>
        <Route path="/" element={<StatusPage />} />
        <Route
          path="/servers"
          element={
            <RequireAuth>
              <ServersPage />
            </RequireAuth>
          }
        />
        <Route
          path="/servers/:serverId"
          element={
            <RequireAuth>
              <ServerDetailPage />
            </RequireAuth>
          }
        />
        <Route
          path="/servers/:serverId/monitors/:monitorId"
          element={
            <RequireAuth>
              <MonitorDetailPage />
            </RequireAuth>
          }
        />
        <Route
          path="/monitor-groups"
          element={
            <RequireAuth>
              <MonitorGroupsPage />
            </RequireAuth>
          }
        />
        <Route
          path="/monitor-groups/:groupId"
          element={
            <RequireAuth>
              <MonitorGroupDetailPage />
            </RequireAuth>
          }
        />
        <Route
          path="/mcp"
          element={
            <RequireAuth>
              <McpPage />
            </RequireAuth>
          }
        />
        <Route
          path="/settings"
          element={
            <RequireAuth>
              <SettingsPage />
            </RequireAuth>
          }
        />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
