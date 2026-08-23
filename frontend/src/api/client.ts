import axios, { AxiosInstance } from 'axios'
import type {
  Cluster,
  Namespace,
  Node,
  NodeMetric,
  Workload,
  HelmRelease,
  UpdateFinding,
  FindingSummary,
  OverviewData,
  PaginatedResponse,
  IntegrationAccount,
  FindingFilter,
  WorkloadFilter,
  NodeFilter,
  HelmFilter,
  SecretFilter,
  Secret,
  FindingStatus,
  LoginResponse,
  User,
  ImageRegistry,
  RegistryTestResult,
  HelmRepository,
  HelmRepositoryTestResult,
  AppSettings,
  SystemResources,
  ManifestApplyResponse,
  RemediateFindingResponse,
  ExceptionRule,
  ActionLog,
  ActionLogFilter,
} from '../types'

const BASE_URL = import.meta.env.VITE_API_URL || '/api/v1'

const api: AxiosInstance = axios.create({
  baseURL: BASE_URL,
  headers: { 'Content-Type': 'application/json' },
})

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('kubepilot_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (res) => res,
  (error) => {
    // A 401 from /auth/login itself just means wrong credentials — that's
    // for the login form to show inline, not a dead session to redirect
    // out of. Redirecting there wiped the "invalid credentials" message
    // with a hard reload before Login.tsx's catch block ever got to render
    // it (found via the Playwright E2E suite exercising this exact case).
    const isLoginRequest = error.config?.url?.includes('/auth/login')
    if (error.response?.status === 401 && !isLoginRequest) {
      localStorage.removeItem('kubepilot_token')
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

// Auth
export async function login(email: string, password: string): Promise<LoginResponse> {
  const { data } = await api.post<LoginResponse>('/auth/login', { email, password })
  return data
}

export async function getMe(): Promise<User> {
  const { data } = await api.get<User>('/auth/me')
  return data
}

// Users (admin)
export async function getUsers(): Promise<User[]> {
  const { data } = await api.get<User[]>('/auth/users')
  return data
}

export async function createUser(payload: {
  email: string
  name: string
  password: string
  role: string
}): Promise<User> {
  const { data } = await api.post<User>('/auth/users', payload)
  return data
}

export async function updateUser(
  id: string,
  payload: { role?: string; is_active?: boolean }
): Promise<User> {
  const { data } = await api.patch<User>(`/auth/users/${id}`, payload)
  return data
}

export async function deleteUser(id: string): Promise<void> {
  await api.delete(`/auth/users/${id}`)
}

// Settings (admin)
export async function getSettings(): Promise<AppSettings> {
  const { data } = await api.get<AppSettings>('/settings')
  return data
}

// Clusters
export async function getClusters(): Promise<Cluster[]> {
  const { data } = await api.get<{ data: Cluster[]; total: number }>('/clusters')
  return data.data
}

export async function getCluster(id: string): Promise<Cluster> {
  const { data } = await api.get<Cluster>(`/clusters/${id}`)
  return data
}

export interface CreateClusterPayload {
  name: string
  environment_id?: string
  provider?: string
  region?: string
  api_endpoint?: string
  kubeconfig_ref?: string
  tls_insecure?: boolean
  connection_mode?: 'kubeconfig' | 'ssh'
  ssh_host?: string
  ssh_port?: number
  ssh_user?: string
  ssh_password?: string
  ssh_kubeconfig_path?: string
  ssh_sudo?: boolean
}

export async function createCluster(payload: CreateClusterPayload): Promise<Cluster> {
  const { data } = await api.post<Cluster>('/clusters', payload)
  return data
}

export async function getClusterResources(id: string): Promise<SystemResources> {
  const { data } = await api.get<SystemResources>(`/clusters/${id}/resources`)
  return data
}

export async function syncCluster(id: string): Promise<void> {
  await api.post(`/clusters/${id}/sync`)
}

export async function deleteCluster(id: string): Promise<void> {
  await api.delete(`/clusters/${id}`)
}

// Namespaces
export async function getNamespaces(clusterId?: string): Promise<Namespace[]> {
  const { data } = await api.get<Namespace[]>('/namespaces', {
    params: clusterId ? { cluster_id: clusterId } : undefined,
  })
  return data
}

// Nodes
export async function getNodes(filter?: NodeFilter): Promise<PaginatedResponse<Node>> {
  const { data } = await api.get<PaginatedResponse<Node>>('/nodes', { params: filter })
  return data
}

export async function getNode(id: string): Promise<Node> {
  const { data } = await api.get<Node>(`/nodes/${id}`)
  return data
}

export async function getNodeMetrics(id: string, since?: string): Promise<NodeMetric[]> {
  const { data } = await api.get<PaginatedResponse<NodeMetric>>(`/nodes/${id}/metrics`, {
    params: since ? { since } : undefined,
  })
  return data.data
}

// Workloads
export async function getWorkloads(filter?: WorkloadFilter): Promise<PaginatedResponse<Workload>> {
  const { data } = await api.get<PaginatedResponse<Workload>>('/workloads', { params: filter })
  return data
}

export async function getWorkload(id: string): Promise<Workload> {
  const { data } = await api.get<Workload>(`/workloads/${id}`)
  return data
}

export async function scaleWorkload(id: string, replicas: number): Promise<{ id: string; replicas: number }> {
  const { data } = await api.patch<{ id: string; replicas: number }>(`/workloads/${id}/scale`, { replicas })
  return data
}

export async function restartWorkload(id: string): Promise<void> {
  await api.post(`/workloads/${id}/restart`)
}

// Helm releases
export async function getHelmReleases(filter?: HelmFilter): Promise<PaginatedResponse<HelmRelease>> {
  const { data } = await api.get<PaginatedResponse<HelmRelease>>('/helm', { params: filter })
  return data
}

export async function getHelmRelease(id: string): Promise<HelmRelease> {
  const { data } = await api.get<HelmRelease>(`/helm/${id}`)
  return data
}

export interface UpgradeHelmReleasePayload {
  chart_version?: string
  values?: Record<string, any>
}

export async function upgradeHelmRelease(id: string, payload: UpgradeHelmReleasePayload): Promise<{ id: string; revision: number; chart_version: string }> {
  const { data } = await api.post(`/helm/${id}/upgrade`, payload)
  return data
}

export async function rollbackHelmRelease(id: string, revision: number): Promise<{ id: string; revision: number }> {
  const { data } = await api.post(`/helm/${id}/rollback`, { revision })
  return data
}

// Raw manifest deploy (server-side apply).
export interface ApplyManifestPayload {
  manifest: string
  namespace?: string
  dry_run?: boolean
  force?: boolean
}

export async function applyManifest(clusterId: string, payload: ApplyManifestPayload): Promise<ManifestApplyResponse> {
  const { data } = await api.post<ManifestApplyResponse>(`/clusters/${clusterId}/manifests/apply`, payload)
  return data
}

// Findings
export async function getFindings(filter?: FindingFilter): Promise<PaginatedResponse<UpdateFinding>> {
  const { data } = await api.get<PaginatedResponse<UpdateFinding>>('/findings', { params: filter })
  return data
}

export async function getFinding(id: string): Promise<UpdateFinding> {
  const { data } = await api.get<UpdateFinding>(`/findings/${id}`)
  return data
}

export async function updateFindingStatus(
  id: string,
  status: FindingStatus,
  meta?: { planned_date?: string; reason?: string }
): Promise<UpdateFinding> {
  const { data } = await api.patch<UpdateFinding>(`/findings/${id}/status`, { status, ...meta })
  return data
}

export async function getFindingSummary(): Promise<FindingSummary> {
  const { data } = await api.get<FindingSummary>('/findings/summary')
  return data
}

// Applies the real fix a finding represents (image tag bump or Helm
// upgrade) — unlike updateFindingStatus, this mutates the cluster.
export async function remediateFinding(id: string): Promise<RemediateFindingResponse> {
  const { data } = await api.post<RemediateFindingResponse>(`/findings/${id}/remediate`)
  return data
}

// Downloads the current findings selection as a CSV file. Mirrors the server-
// side filters that ListFindings understands (cluster_id, single severity/status).
export async function exportFindingsCsv(filter?: FindingFilter): Promise<void> {
  const params: Record<string, string> = {}
  if (filter?.cluster_id) params.cluster_id = filter.cluster_id
  if (Array.isArray(filter?.severity) && filter!.severity!.length === 1) params.severity = filter!.severity![0]
  if (Array.isArray(filter?.status) && filter!.status!.length === 1) params.status = filter!.status![0]

  const res = await api.get('/findings/export', { params, responseType: 'blob' })
  const url = window.URL.createObjectURL(res.data as Blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `kubepilot-findings-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(a)
  a.click()
  a.remove()
  window.URL.revokeObjectURL(url)
}

// Secrets
export async function getSecrets(filter?: SecretFilter): Promise<PaginatedResponse<Secret>> {
  const { data } = await api.get<PaginatedResponse<Secret>>('/secrets', { params: filter })
  return data
}

// Overview
export async function getOverview(): Promise<OverviewData> {
  const { data } = await api.get<OverviewData>('/overview')
  return data
}

// Integrations
export async function getIntegrations(): Promise<IntegrationAccount[]> {
  const { data } = await api.get<IntegrationAccount[]>('/integrations')
  return data
}

export async function createIntegration(payload: {
  name: string
  integration_type: string
  url?: string
  credentials_ref?: string
}): Promise<IntegrationAccount> {
  const { data } = await api.post<IntegrationAccount>('/integrations', payload)
  return data
}

export async function testIntegration(id: string): Promise<{ success: boolean; message: string }> {
  const { data } = await api.post<{ success: boolean; message: string }>(`/integrations/${id}/test`)
  return data
}

export async function deleteIntegration(id: string): Promise<void> {
  await api.delete(`/integrations/${id}`)
}

// Image registries
export interface RegistryPayload {
  name: string
  host: string
  type?: string
  username?: string
  password?: string
  tls_insecure?: boolean
  rate_limit_rpm?: number
}

export async function getRegistries(): Promise<ImageRegistry[]> {
  const { data } = await api.get<ImageRegistry[]>('/registries')
  return data
}

export async function createRegistry(payload: RegistryPayload): Promise<ImageRegistry> {
  const { data } = await api.post<ImageRegistry>('/registries', payload)
  return data
}

export async function updateRegistry(id: string, payload: RegistryPayload): Promise<ImageRegistry> {
  const { data } = await api.put<ImageRegistry>(`/registries/${id}`, payload)
  return data
}

export async function testRegistry(id: string): Promise<RegistryTestResult> {
  const { data } = await api.post<RegistryTestResult>(`/registries/${id}/test`)
  return data
}

export async function deleteRegistry(id: string): Promise<void> {
  await api.delete(`/registries/${id}`)
}

// Helm chart repositories — used to resolve which repository an installed chart
// came from, since a Helm release does not record its own origin.
export interface HelmRepositoryPayload {
  name: string
  url: string
  username?: string
  password?: string
  tls_insecure?: boolean
}

export async function getHelmRepositories(): Promise<HelmRepository[]> {
  const { data } = await api.get<HelmRepository[]>('/helm-repositories')
  return data
}

export async function createHelmRepository(payload: HelmRepositoryPayload): Promise<HelmRepository> {
  const { data } = await api.post<HelmRepository>('/helm-repositories', payload)
  return data
}

export async function updateHelmRepository(id: string, payload: HelmRepositoryPayload): Promise<HelmRepository> {
  const { data } = await api.put<HelmRepository>(`/helm-repositories/${id}`, payload)
  return data
}

export async function testHelmRepository(id: string): Promise<HelmRepositoryTestResult> {
  const { data } = await api.post<HelmRepositoryTestResult>(`/helm-repositories/${id}/test`)
  return data
}

export async function deleteHelmRepository(id: string): Promise<void> {
  await api.delete(`/helm-repositories/${id}`)
}

// Exception rules — change how the scoring engine treats matching findings
// (suppress / reduce severity / accept risk). Scope is whichever of
// cluster_id, workload_id or image_pattern is set; namespace_name narrows a
// cluster-scoped rule further. None set = global.
export interface ExceptionRulePayload {
  name: string
  rule_type: string
  cluster_id?: string
  namespace_name?: string
  workload_id?: string
  finding_kind?: string
  image_pattern?: string
  reason: string
  expires_at?: string | null
  is_active?: boolean
}

export async function getExceptionRules(): Promise<ExceptionRule[]> {
  const { data } = await api.get<ExceptionRule[]>('/exception-rules')
  return data
}

export async function createExceptionRule(payload: ExceptionRulePayload): Promise<ExceptionRule> {
  const { data } = await api.post<ExceptionRule>('/exception-rules', payload)
  return data
}

export async function updateExceptionRule(id: string, payload: ExceptionRulePayload): Promise<ExceptionRule> {
  const { data } = await api.put<ExceptionRule>(`/exception-rules/${id}`, payload)
  return data
}

export async function deleteExceptionRule(id: string): Promise<void> {
  await api.delete(`/exception-rules/${id}`)
}

// Action logs — audit trail of every write action KubePilot has taken.
export async function getActionLogs(filter?: ActionLogFilter): Promise<PaginatedResponse<ActionLog>> {
  const { data } = await api.get<PaginatedResponse<ActionLog>>('/action-logs', { params: filter })
  return data
}

export default api
