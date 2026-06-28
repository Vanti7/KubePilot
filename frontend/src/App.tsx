import { Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './contexts/AuthContext'
import { ClusterProvider } from './contexts/ClusterContext'
import { Layout } from './components/Layout'
import { LoginPage } from './pages/Login'
import { Overview } from './pages/Overview'
import { Updates } from './pages/Updates'
import { Risks } from './pages/Risks'
import { Inventory } from './pages/Inventory'
import { HelmPage } from './pages/HelmPage'
import { Nodes } from './pages/Nodes'
import { Integrations } from './pages/Integrations'
import { Secrets } from './pages/Secrets'
import { Clusters } from './pages/Clusters'
import { ClusterDetail } from './pages/ClusterDetail'
import { Registries } from './pages/Registries'

function ProtectedRoutes() {
  const { token, isLoading } = useAuth()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-screen bg-surface-base">
        <div className="flex flex-col items-center gap-3">
          <div className="w-6 h-6 rounded bg-blue-600 flex items-center justify-center animate-pulse">
            <span className="text-white text-xs font-bold">K</span>
          </div>
          <p className="text-sm text-slate-500">Loading...</p>
        </div>
      </div>
    )
  }

  if (!token) {
    return <Navigate to="/login" replace />
  }

  return (
    <ClusterProvider>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<Overview />} />
          <Route path="/updates" element={<Updates />} />
          <Route path="/risks" element={<Risks />} />
          <Route path="/inventory" element={<Inventory />} />
          <Route path="/helm" element={<HelmPage />} />
          <Route path="/nodes" element={<Nodes />} />
          <Route path="/secrets" element={<Secrets />} />
          <Route path="/registries" element={<Registries />} />
          <Route path="/integrations" element={<Integrations />} />
          <Route path="/clusters" element={<Clusters />} />
          <Route path="/clusters/:id" element={<ClusterDetail />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </ClusterProvider>
  )
}

export default function App() {
  return (
    <AuthProvider>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/*" element={<ProtectedRoutes />} />
      </Routes>
    </AuthProvider>
  )
}
