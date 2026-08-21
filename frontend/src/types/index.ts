export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info'
export type FindingStatus = 'open' | 'planned' | 'ignored' | 'approved' | 'blocked' | 'resolved'
export type UpdateType = 'patch' | 'minor' | 'major' | 'unknown'
export type ClusterStatus = 'healthy' | 'unreachable' | 'degraded' | 'unknown'

export interface Environment {
  id: string
  name: string
  slug: string
  criticality_weight: number
  color: string
}

export type ClusterConnectionMode = 'kubeconfig' | 'incluster' | 'ssh'

export interface Cluster {
  id: string
  name: string
  slug?: string
  // Legacy/optional fields kept for backward compatibility with existing views.
  display_name?: string
  endpoint?: string
  version?: string
  provider?: string
  region?: string
  k8s_version?: string
  api_endpoint?: string
  tls_insecure?: boolean
  // Connection mode and SSH parameters (ssh_password is never returned by the API).
  connection_mode?: ClusterConnectionMode
  ssh_host?: string
  ssh_port?: number
  ssh_user?: string
  ssh_sudo?: boolean
  ssh_kubeconfig_path?: string
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
  metrics?: NodeMetricSnapshot
  last_seen_at: string
}

// Latest usage snapshot embedded in the node list.
export interface NodeMetricSnapshot {
  timestamp: string
  cpu_usage_percent: number
  memory_usage_percent: number
  fs_used_percent: number
  network_rx_rate: number
  network_tx_rate: number
  pods_running: number
}

// A full point in a node's usage time-series (GET /nodes/:id/metrics).
export interface NodeMetric {
  id: string
  cluster_id: string
  node_id: string
  node_name: string
  timestamp: string
  cpu_usage_nano_cores: number
  cpu_usage_percent: number
  memory_working_set_bytes: number
  memory_usage_bytes: number
  memory_usage_percent: number
  fs_used_bytes: number
  fs_capacity_bytes: number
  fs_used_percent: number
  network_rx_bytes: number
  network_tx_bytes: number
  network_rx_rate: number
  network_tx_rate: number
  pods_running: number
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
  cluster?: { id: string; name: string; slug: string }
  namespace_name: string
  name: string
  chart_name: string
  chart_version: string
  app_version?: string
  repo_url?: string
  status: string
  revision: number
  values: Record<string, any>
  last_deployed_at?: string
  last_seen_at: string
}

// Mirrors store.FindingWithScore — the flat DTO returned by GET /api/v1/findings.
export interface UpdateFinding {
  id: string
  kind: string // finding type: 'image' | 'helm'
  cluster_id: string
  workload_id?: string
  container_image_id?: string
  helm_release_id?: string
  title: string
  description?: string
  current_version: string
  latest_version: string
  update_type: UpdateType
  severity: Severity
  status: FindingStatus
  first_detected_at: string
  last_observed_at: string
  cves?: string[]
  metadata?: Record<string, any>
  score?: number
  score_severity?: Severity
  score_factors?: Record<string, number>
  workload_name?: string
  workload_kind?: string // Kubernetes kind: Deployment | DaemonSet | StatefulSet
  helm_release_name?: string // set for helm findings, which have no workload
  cluster_name?: string
  namespace_name?: string
  // Set for image findings only — needed to show/build the "Fix now" target
  // image reference (registry/repository:latest_version); the finding row
  // itself only carries the tag (latest_version), not the rest of the ref.
  container_name?: string
  image_registry?: string
  image_repository?: string
}

export interface RemediateFindingResponse {
  id: string
  kind: string
  detail: Record<string, any>
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
  status: ClusterStatus
  open: number
  critical: number
  high: number
  medium: number
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
  system_resources: SystemResources
  resources_per_cluster: ClusterResources[]
  generated_at: string
}

// Cluster-wide capacity vs live usage (dashboard system view).
export interface SystemResources {
  nodes: number
  nodes_ready: number
  pods_running: number
  cpu_capacity_cores: number
  cpu_used_cores: number
  cpu_usage_percent: number
  memory_capacity_bytes: number
  memory_used_bytes: number
  memory_usage_percent: number
  disk_capacity_bytes: number
  disk_used_bytes: number
  disk_usage_percent: number
}

export interface ClusterResources extends SystemResources {
  cluster_id: string
  cluster_name: string
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

export type RegistryType = 'generic' | 'dockerhub' | 'harbor' | 'ecr' | 'gcr' | 'acr' | 'ghcr' | 'quay'

export interface ImageRegistry {
  id: string
  name: string
  host: string
  type: RegistryType
  username?: string
  has_credentials: boolean
  tls_insecure: boolean
  rate_limit_rpm: number
  created_at: string
  updated_at: string
}

export interface RegistryTestResult {
  ok: boolean
  status_code: number
  message: string
}

export interface HelmRepository {
  id: string
  name: string
  url: string
  username?: string
  has_credentials: boolean
  tls_insecure: boolean
  built_in: boolean
  created_at: string
  updated_at: string
}

export interface HelmRepositoryTestResult {
  ok: boolean
  charts: number
  message: string
}

export type ExceptionRuleType = 'suppress' | 'reduce_severity' | 'accept_risk'

export interface ExceptionRule {
  id: string
  name: string
  rule_type: ExceptionRuleType
  cluster_id?: string
  cluster?: { id: string; name: string }
  namespace_name?: string
  workload_id?: string
  workload?: { id: string; name: string }
  finding_kind?: string
  image_pattern?: string
  reason: string
  expires_at?: string
  created_by_id?: string
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface FindingFilter {
  severity?: Severity[]
  status?: FindingStatus[]
  update_type?: UpdateType[]
  kind?: string
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

export interface Secret {
  id: string
  cluster_id: string
  namespace_name: string
  name: string
  type: string
  keys: string[]
  k8s_created_at?: string
  k8s_updated_at?: string
  last_seen_at: string
  cluster_name?: string
}

export interface SecretFilter {
  cluster_id?: string
  namespace?: string
  type?: string
  limit?: number
  offset?: number
}

export interface ManifestApplyResult {
  kind: string
  name: string
  namespace?: string
  operation: 'created' | 'updated' | 'error'
  error?: string
}

export interface ManifestApplyResponse {
  results: ManifestApplyResult[]
  dry_run: boolean
  ok: boolean
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
  is_active?: boolean
  last_login_at?: string | null
  created_at?: string
}

export interface AppSettings {
  version: string
  storage_driver: string
  cache_driver: string
  local_mode: boolean
  demo_mode: boolean
  in_cluster: boolean
  worker_interval_seconds: number
  node_metrics_enabled: boolean
  node_metrics_retention_hours: number
  mcp_allow_writes: boolean
}

export interface ActionLog {
  id: string
  user_id?: string
  user?: Pick<User, 'id' | 'name' | 'email'>
  action: string
  entity_type: string
  entity_id: string
  status: 'success' | 'failure'
  details?: Record<string, any>
  ip_address?: string
  user_agent?: string
  created_at: string
}

export interface ActionLogFilter {
  user_id?: string
  action?: string
  entity_type?: string
  entity_id?: string
  status?: string
  limit?: number
  offset?: number
}
