import { Navigate, Route, Routes } from "react-router-dom"

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

function LoadingScreen() {
  return (
    <div className="flex min-h-[100dvh] items-center justify-center text-muted-foreground">
      加载中…
    </div>
  )
}

export function App() {
  const { loading } = useAuth()
  if (loading) return <LoadingScreen />

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<Layout />}>
        <Route path="/" element={<StatusPage />} />
        <Route path="/servers" element={<ServersPage />} />
        <Route path="/servers/:serverId" element={<ServerDetailPage />} />
        <Route
          path="/servers/:serverId/monitors/:monitorId"
          element={<MonitorDetailPage />}
        />
        <Route path="/monitor-groups" element={<MonitorGroupsPage />} />
        <Route
          path="/monitor-groups/:groupId"
          element={<MonitorGroupDetailPage />}
        />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
