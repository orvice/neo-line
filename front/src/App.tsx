import { Navigate, Route, Routes } from "react-router-dom"

import { useAuth } from "./lib/auth"
import { Layout } from "./components/layout"
import { LoginPage } from "./pages/login"
import { ServersPage } from "./pages/servers"
import { ServerDetailPage } from "./pages/server-detail"
import { MonitorDetailPage } from "./pages/monitor-detail"

function LoadingScreen() {
  return (
    <div className="flex h-screen items-center justify-center text-muted-foreground">
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
        <Route path="/" element={<ServersPage />} />
        <Route path="/servers/:serverId" element={<ServerDetailPage />} />
        <Route
          path="/servers/:serverId/monitors/:monitorId"
          element={<MonitorDetailPage />}
        />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
