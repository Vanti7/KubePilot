import axios, { AxiosInstance } from 'axios'
import type {
  Cluster,
  Namespace,
  Node,
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
  FindingStatus,
  LoginResponse,
  User,
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
    if (error.response?.status === 401) {
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

// Clusters
export async function getClusters(): Promise<Cluster[]> {
  const { data } = await api.get<Cluster[]>('/clusters')
  return data
}

export async function getCluster(id: string): Promise<Cluster> {
  const { data } = await api.get<Cluster>(`/clusters/${id}`)
  return data
}

export async function createCluster(payload: {
  name: string
  display_name: string
  endpoint: string
  kubeconfig?: string
  environment_id: string
}): Promise<Cluster> {
  const { data } = await api.post<Cluster>('/clusters', payload)
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

// Workloads
export async function getWorkloads(filter?: WorkloadFilter): Promise<PaginatedResponse<Workload>> {
  const { data } = await api.get<PaginatedResponse<Workload>>('/workloads', { params: filter })
  return data
}

export async function getWorkload(id: string): Promise<Workload> {
  const { data } = await api.get<Workload>(`/workloads/${id}`)
  return data
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

export default api
