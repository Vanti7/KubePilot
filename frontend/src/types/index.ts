export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info'
export type FindingStatus = 'open' | 'planned' | 'ignored' | 'approved' | 'blocked' | 'resolved'
export type UpdateType = 'patch' | 'minor' | 'major' | 'unknown'
export type ClusterStatus = 'healthy' | 'unreachable' | 'degraded'

export interface Environment {
  id: string
  name: string
  slug: string
  criticality_weight: number
  color: string
}

export interface Cluster {
  id: string
  name: string
  display_name: string
  endpoint: string
  version: string
  environment_id: string
  environment?: Environment
  last_seen_at: string
  status: ClusterStatus
}

export interface Namespace {
  id: string
  cluster_id: string
  name: string
  phase: string
}

export interface Node {
  id: string
  cluster_id: string
  name: string
  role: string
  os_image: string
  kernel_version: string
  kubelet_version: string
  container_runtime: string
  capacity: Record<string, string>
  allocatable: Record<string, string>
  conditions: NodeCondition[]
  last_seen_at: string
}

export interface NodeCondition {
  type: string
  status: string
  reason?: string
  message?: string
}

export interface Workload {
  id: string
  cluster_id: string
  namespace_id: string
  name: string
  kind: string
  uid: string
  replicas_desired: number
  replicas_ready: number
  labels: Record<string, string>
  last_observed_at: string
  cluster_name?: string
  namespace_name?: string
  open_findings_count?: number
}

export interface ContainerImage {
  id: string
  workload_id: string
  container_name: string
  image_ref: string
  registry: string
  repository: string
  tag: string
  digest: string
}

export interface HelmRelease {
  id: string
  cluster_id: string
  namespace_id: string
  release_name: string
  chart_name: string
  chart_version: string
  app_version: string
  repo_url: string
  status: string
  last_deployed_at: string
  cluster_name?: string
  namespace_name?: string
  available_version?: string
}

export interface UpdateFinding {
  id: string
  kind: string
  target_id: string
  target_kind: string
  current_version: string
  latest_version: string
  update_type: UpdateType
  is_breaking: boolean
  changelog_url: string
  status: FindingStatus
  first_detected_at: string
  last_confirmed_at: string
  risk_score?: RiskScore
  workload_name?: string
  cluster_name?: string
  namespace_name?: string
}

export interface RiskScore {
  id: string
  finding_id: string
  score: number
  severity: Severity
  factors: Record<string, number>
  computed_at: string
}

export interface FindingSummary {
  total: number
  critical: number
  high: number
  medium: number
  low: number
  info: number
}

export interface FindingsPerCluster {
  cluster_id: string
  cluster_name: string
  open: number
  critical: number
}

export interface OverviewData {
  cluster_status: {
    total: number
    healthy: number
    degraded: number
    unreachable: number
    unknown: number
  }
  findings_summary: FindingSummary
  findings_per_cluster: FindingsPerCluster[]
  top_findings: UpdateFinding[]
  data_freshness: Array<{
    cluster_id: string
    cluster_name: string
    last_seen_at: string | null
    age_seconds: number
    status: string
  }>
  generated_at: string
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  limit: number
  offset: number
}

export interface IntegrationAccount {
  id: string
  name: string
  integration_type: string
  enabled: boolean
  last_tested_at: string
  last_test_status: string
}

export interface FindingFilter {
  severity?: Severity[]
  status?: FindingStatus[]
  update_type?: UpdateType[]
  cluster_id?: string
  namespace?: string
  limit?: number
  offset?: number
  sort_by?: string
  sort_dir?: 'asc' | 'desc'
}

export interface WorkloadFilter {
  cluster_id?: string
  namespace_id?: string
  kind?: string
  limit?: number
  offset?: number
}

export interface NodeFilter {
  cluster_id?: string
  role?: string
  limit?: number
  offset?: number
}

export interface HelmFilter {
  cluster_id?: string
  namespace_id?: string
  has_updates?: boolean
  limit?: number
  offset?: number
}

export interface LoginResponse {
  token: string
  user: User
}

export interface User {
  id: string
  email: string
  name: string
  role: string
}
