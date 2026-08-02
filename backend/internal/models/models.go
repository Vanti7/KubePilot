package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// ---------------------------------------------------------------------------
// Enum constants
// ---------------------------------------------------------------------------

// Cluster status
const (
	ClusterStatusUnknown     = "unknown"
	ClusterStatusHealthy     = "healthy"
	ClusterStatusDegraded    = "degraded"
	ClusterStatusUnreachable = "unreachable"
)

// Cluster connection mode — how the collector reaches the Kubernetes API.
const (
	ClusterConnKubeconfig = "kubeconfig" // KubeconfigRef holds the kubeconfig
	ClusterConnInCluster  = "incluster"  // pod service-account token
	ClusterConnSSH        = "ssh"        // SSH to a node, read its kubeconfig, tunnel API traffic
)

// Workload kind
const (
	WorkloadKindDeployment  = "Deployment"
	WorkloadKindDaemonSet   = "DaemonSet"
	WorkloadKindStatefulSet = "StatefulSet"
)

// Workload health
const (
	WorkloadHealthHealthy  = "healthy"
	WorkloadHealthDegraded = "degraded"
	WorkloadHealthWarning  = "warning"
	WorkloadHealthUnknown  = "unknown"
)

// UpdateFinding kind
const (
	FindingKindImage = "image"
	FindingKindHelm  = "helm"
	FindingKindNode  = "node"
)

// UpdateFinding update type
const (
	UpdateTypePatch   = "patch"
	UpdateTypeMinor   = "minor"
	UpdateTypeMajor   = "major"
	UpdateTypeUnknown = "unknown"
)

// UpdateFinding severity
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
	SeverityInfo     = "info"
)

// UpdateFinding status
const (
	FindingStatusOpen     = "open"
	FindingStatusPlanned  = "planned"
	FindingStatusIgnored  = "ignored"
	FindingStatusApproved = "approved"
	FindingStatusBlocked  = "blocked"
	FindingStatusResolved = "resolved"
)

// Action log action
const (
	ActionLogCreate       = "create"
	ActionLogUpdate       = "update"
	ActionLogDelete       = "delete"
	ActionLogUpdateStatus = "update_status"
	ActionLogResolve      = "resolve"
	ActionLogIgnore       = "ignore"
	ActionLogSync         = "sync"
	ActionLogScale        = "scale"
	ActionLogRestart      = "restart"
	ActionLogHelmUpgrade  = "helm_upgrade"
	ActionLogHelmRollback = "helm_rollback"
)

// Action log outcome
const (
	ActionLogStatusSuccess = "success"
	ActionLogStatusFailure = "failure"
)

// User roles
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// Secret types
const (
	SecretTypeOpaque              = "Opaque"
	SecretTypeTLS                 = "kubernetes.io/tls"
	SecretTypeDockerConfigJSON    = "kubernetes.io/dockerconfigjson"
	SecretTypeServiceAccountToken = "kubernetes.io/service-account-token"
	SecretTypeBasicAuth           = "kubernetes.io/basic-auth"
)

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

// Environment represents a deployment environment (prod, staging, dev, etc.).
type Environment struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name              string    `gorm:"uniqueIndex;not null"                           json:"name"`
	Slug              string    `gorm:"uniqueIndex;not null"                           json:"slug"`
	CriticalityWeight float64   `gorm:"default:1.0"                                    json:"criticality_weight"`
	Color             string    `gorm:"default:'#6B7280'"                              json:"color"`
	CreatedAt         time.Time `                                                      json:"created_at"`
	UpdatedAt         time.Time `                                                      json:"updated_at"`
}

// Cluster represents a Kubernetes cluster registered in KubePilot.
type Cluster struct {
	ID            uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	EnvironmentID *uuid.UUID   `gorm:"type:uuid;index"                                json:"environment_id,omitempty"`
	Environment   *Environment `gorm:"foreignKey:EnvironmentID"                       json:"environment,omitempty"`
	Name          string       `gorm:"uniqueIndex;not null"                           json:"name"`
	Slug          string       `gorm:"uniqueIndex;not null"                           json:"slug"`
	Provider      string       `gorm:"default:'unknown'"                              json:"provider"`
	Region        string       `                                                      json:"region"`
	K8sVersion    string       `                                                      json:"k8s_version"`
	Status        string       `gorm:"default:'unknown'"                              json:"status"`
	LastSeenAt    *time.Time   `                                                      json:"last_seen_at,omitempty"`
	KubeconfigRef string       `gorm:"column:kubeconfig_ref"                          json:"kubeconfig_ref,omitempty"`
	APIEndpoint   string       `                                                      json:"api_endpoint,omitempty"`
	TLSInsecure   bool         `gorm:"default:false"                                  json:"tls_insecure"`
	// Connection mode and SSH parameters. When ConnectionMode is "ssh", the collector
	// opens an SSH session to SSHHost, reads the kubeconfig from the node, and tunnels
	// all Kubernetes API traffic through that SSH connection. SSHPassword is never
	// serialised back over the API.
	ConnectionMode    string         `gorm:"default:'kubeconfig'"                    json:"connection_mode"`
	SSHHost           string         `                                               json:"ssh_host,omitempty"`
	SSHPort           int            `gorm:"default:22"                              json:"ssh_port,omitempty"`
	SSHUser           string         `                                               json:"ssh_user,omitempty"`
	SSHPassword       string         `gorm:"column:ssh_password"                     json:"-"`
	SSHKubeconfigPath string         `                                               json:"ssh_kubeconfig_path,omitempty"`
	SSHSudo           bool           `gorm:"default:false"                           json:"ssh_sudo"`
	Annotations       datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"annotations,omitempty"`
	Labels            datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"labels,omitempty"`
	CreatedAt         time.Time      `                                                      json:"created_at"`
	UpdatedAt         time.Time      `                                                      json:"updated_at"`
}

// Namespace represents a Kubernetes namespace.
type Namespace struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	ClusterID   uuid.UUID      `gorm:"type:uuid;not null;index"                       json:"cluster_id"`
	Cluster     *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	Name        string         `gorm:"not null"                                       json:"name"`
	Status      string         `gorm:"default:'Active'"                               json:"status"`
	Labels      datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"labels,omitempty"`
	Annotations datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"annotations,omitempty"`
	CreatedAt   time.Time      `                                                      json:"created_at"`
	UpdatedAt   time.Time      `                                                      json:"updated_at"`
}

// Node represents a Kubernetes node.
type Node struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	ClusterID         uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:uq_node,priority:1" json:"cluster_id"`
	Cluster           *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	Name              string         `gorm:"not null;uniqueIndex:uq_node,priority:2"         json:"name"`
	Role              string         `gorm:"default:'worker'"                               json:"role"`
	Status            string         `gorm:"default:'unknown'"                              json:"status"`
	K8sVersion        string         `                                                      json:"k8s_version"`
	OSImage           string         `                                                      json:"os_image"`
	KernelVersion     string         `                                                      json:"kernel_version"`
	ContainerRuntime  string         `                                                      json:"container_runtime"`
	Arch              string         `                                                      json:"arch"`
	CapacityCPU       string         `                                                      json:"capacity_cpu"`
	CapacityMemory    string         `                                                      json:"capacity_memory"`
	AllocatableCPU    string         `                                                      json:"allocatable_cpu"`
	AllocatableMemory string         `                                                      json:"allocatable_memory"`
	Labels            datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"labels,omitempty"`
	Taints            datatypes.JSON `gorm:"type:jsonb;default:'[]'"                        json:"taints,omitempty"`
	Conditions        datatypes.JSON `gorm:"type:jsonb;default:'[]'"                        json:"conditions,omitempty"`
	CreatedAt         time.Time      `                                                      json:"created_at"`
	UpdatedAt         time.Time      `                                                      json:"updated_at"`
}

// NodeMetric is a point-in-time sample of a node's resource usage, collected from
// the kubelet Summary API. It is an append-only time-series — never upserted.
type NodeMetric struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey"                                   json:"id"`
	ClusterID             uuid.UUID `gorm:"type:uuid;not null;index"                              json:"cluster_id"`
	NodeID                uuid.UUID `gorm:"type:uuid;not null;index:idx_node_metric_node_ts,priority:1" json:"node_id"`
	NodeName              string    `gorm:"not null"                                              json:"node_name"`
	Timestamp             time.Time `gorm:"not null;index:idx_node_metric_node_ts,priority:2"     json:"timestamp"`
	CPUUsageNanoCores     int64     `                                                            json:"cpu_usage_nano_cores"`
	CPUUsagePercent       float64   `                                                            json:"cpu_usage_percent"`
	MemoryWorkingSetBytes int64     `                                                            json:"memory_working_set_bytes"`
	MemoryUsageBytes      int64     `                                                            json:"memory_usage_bytes"`
	MemoryUsagePercent    float64   `                                                            json:"memory_usage_percent"`
	FSUsedBytes           int64     `                                                            json:"fs_used_bytes"`
	FSCapacityBytes       int64     `                                                            json:"fs_capacity_bytes"`
	FSUsedPercent         float64   `                                                            json:"fs_used_percent"`
	NetworkRxBytes        int64     `                                                            json:"network_rx_bytes"`
	NetworkTxBytes        int64     `                                                            json:"network_tx_bytes"`
	NetworkRxRate         float64   `                                                            json:"network_rx_rate"`
	NetworkTxRate         float64   `                                                            json:"network_tx_rate"`
	PodsRunning           int       `                                                            json:"pods_running"`
	CreatedAt             time.Time `                                                            json:"created_at"`
}

// Workload represents a Kubernetes workload (Deployment, DaemonSet, StatefulSet).
type Workload struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	ClusterID       uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:uq_workload,priority:1"  json:"cluster_id"`
	Cluster         *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	NamespaceID     *uuid.UUID     `gorm:"type:uuid;index"                                json:"namespace_id,omitempty"`
	Namespace       *Namespace     `gorm:"foreignKey:NamespaceID"                         json:"namespace,omitempty"`
	Name            string         `gorm:"not null;uniqueIndex:uq_workload,priority:3"     json:"name"`
	NamespaceName   string         `gorm:"not null;index;uniqueIndex:uq_workload,priority:2" json:"namespace_name"`
	Kind            string         `gorm:"not null;uniqueIndex:uq_workload,priority:4"     json:"kind"`
	ReplicasDesired int32          `gorm:"default:0"                                      json:"replicas_desired"`
	ReplicasReady   int32          `gorm:"default:0"                                      json:"replicas_ready"`
	HealthStatus    string         `gorm:"default:'unknown'"                              json:"health_status"`
	Labels          datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"labels,omitempty"`
	Annotations     datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"annotations,omitempty"`
	LastSeenAt      time.Time      `                                                      json:"last_seen_at"`
	CreatedAt       time.Time      `                                                      json:"created_at"`
	UpdatedAt       time.Time      `                                                      json:"updated_at"`
}

// WorkloadSource links a workload to its Helm release (if applicable).
type WorkloadSource struct {
	ID            uuid.UUID    `gorm:"type:uuid;primaryKey" json:"id"`
	WorkloadID    uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:idx_workload_source" json:"workload_id"`
	Workload      *Workload    `gorm:"foreignKey:WorkloadID"                              json:"workload,omitempty"`
	HelmReleaseID *uuid.UUID   `gorm:"type:uuid;index"                                    json:"helm_release_id,omitempty"`
	HelmRelease   *HelmRelease `gorm:"foreignKey:HelmReleaseID"                         json:"helm_release,omitempty"`
	SourceType    string       `gorm:"default:'unknown'"                                  json:"source_type"`
	CreatedAt     time.Time    `                                                          json:"created_at"`
	UpdatedAt     time.Time    `                                                          json:"updated_at"`
}

// ContainerImage represents a container image used by a workload.
type ContainerImage struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	WorkloadID      uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:uq_container_image,priority:1" json:"workload_id"`
	Workload        *Workload      `gorm:"foreignKey:WorkloadID"                          json:"workload,omitempty"`
	ContainerName   string         `gorm:"not null;uniqueIndex:uq_container_image,priority:2" json:"container_name"`
	Image           string         `gorm:"not null"                                       json:"image"`
	Registry        string         `gorm:"not null"                                       json:"registry"`
	Repository      string         `gorm:"not null"                                       json:"repository"`
	Tag             string         `gorm:"not null"                                       json:"tag"`
	Digest          string         `                                                      json:"digest,omitempty"`
	IsInitContainer bool           `gorm:"default:false"                                  json:"is_init_container"`
	RegistryID      *uuid.UUID     `gorm:"type:uuid;index"                                json:"registry_id,omitempty"`
	ImageRegistry   *ImageRegistry `gorm:"foreignKey:RegistryID"                      json:"image_registry,omitempty"`
	CreatedAt       time.Time      `                                                      json:"created_at"`
	UpdatedAt       time.Time      `                                                      json:"updated_at"`
}

// ImageRegistry stores registry configuration and credentials.
type ImageRegistry struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Name           string         `gorm:"uniqueIndex;not null"                           json:"name"`
	Host           string         `gorm:"uniqueIndex;not null"                           json:"host"`
	Type           string         `gorm:"default:'generic'"                              json:"type"`
	CredentialsRef string         `                                                      json:"credentials_ref,omitempty"`
	AuthConfig     datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"-"`
	TLSInsecure    bool           `gorm:"default:false"                                  json:"tls_insecure"`
	RateLimitRPM   int            `gorm:"default:60"                                     json:"rate_limit_rpm"`
	CreatedAt      time.Time      `                                                      json:"created_at"`
	UpdatedAt      time.Time      `                                                      json:"updated_at"`
}

// HelmRepository is a chart repository the Helm watcher searches to resolve the
// origin of an installed release. Helm 3 release secrets record the chart name
// and version but not the repository it came from, so the link has to be
// rebuilt by looking the chart up in known repositories.
type HelmRepository struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Name        string         `gorm:"uniqueIndex;not null"                           json:"name"`
	URL         string         `gorm:"uniqueIndex;not null"                           json:"url"`
	AuthConfig  datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"-"`
	TLSInsecure bool           `gorm:"default:false"                                  json:"tls_insecure"`
	BuiltIn     bool           `gorm:"default:false"                                  json:"built_in"`
	CreatedAt   time.Time      `                                                      json:"created_at"`
	UpdatedAt   time.Time      `                                                      json:"updated_at"`
}

// ImageTagObservation records a tag observed in a registry at a point in time.
type ImageTagObservation struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey" json:"id"`
	ImageID     uuid.UUID       `gorm:"type:uuid;not null;index;uniqueIndex:uq_image_tag,priority:1" json:"image_id"`
	Image       *ContainerImage `gorm:"foreignKey:ImageID"                        json:"image,omitempty"`
	Tag         string          `gorm:"not null;uniqueIndex:uq_image_tag,priority:2"   json:"tag"`
	Digest      string          `                                                      json:"digest,omitempty"`
	PublishedAt *time.Time      `                                                      json:"published_at,omitempty"`
	ObservedAt  time.Time       `gorm:"not null"                                       json:"observed_at"`
	IsLatest    bool            `gorm:"default:false"                                  json:"is_latest"`
}

// Secret represents a Kubernetes Secret (metadata only — values are never stored).
type Secret struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	ClusterID     uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:uq_secret,priority:1" json:"cluster_id"`
	Cluster       *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	NamespaceName string         `gorm:"not null;index;uniqueIndex:uq_secret,priority:2" json:"namespace_name"`
	Name          string         `gorm:"not null;uniqueIndex:uq_secret,priority:3"       json:"name"`
	Type          string         `gorm:"not null;default:'Opaque'"                      json:"type"`
	Keys          datatypes.JSON `gorm:"type:jsonb;default:'[]'"                        json:"keys"`
	K8sCreatedAt  *time.Time     `                                                      json:"k8s_created_at,omitempty"`
	K8sUpdatedAt  *time.Time     `                                                      json:"k8s_updated_at,omitempty"`
	LastSeenAt    time.Time      `                                                      json:"last_seen_at"`
	CreatedAt     time.Time      `                                                      json:"created_at"`
	UpdatedAt     time.Time      `                                                      json:"updated_at"`
}

// HelmRelease represents a Helm release deployed in a cluster.
type HelmRelease struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	ClusterID      uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:uq_helm_release,priority:1" json:"cluster_id"`
	Cluster        *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	NamespaceName  string         `gorm:"not null;index;uniqueIndex:uq_helm_release,priority:2" json:"namespace_name"`
	Name           string         `gorm:"not null;uniqueIndex:uq_helm_release,priority:3" json:"name"`
	ChartName      string         `gorm:"not null"                                       json:"chart_name"`
	ChartVersion   string         `gorm:"not null"                                       json:"chart_version"`
	AppVersion     string         `                                                      json:"app_version,omitempty"`
	RepoURL        string         `                                                      json:"repo_url,omitempty"`
	Status         string         `gorm:"default:'deployed'"                             json:"status"`
	Revision       int            `gorm:"default:1"                                      json:"revision"`
	Values         datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"values,omitempty"`
	LastDeployedAt *time.Time     `                                                      json:"last_deployed_at,omitempty"`
	LastSeenAt     time.Time      `                                                      json:"last_seen_at"`
	CreatedAt      time.Time      `                                                      json:"created_at"`
	UpdatedAt      time.Time      `                                                      json:"updated_at"`
}

// UpdateFinding represents a detected update opportunity or security finding.
type UpdateFinding struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	// ClusterID participates in two composite unique indexes:
	//   uq_finding_by_image  (cluster_id, container_image_id) — one finding per container image per cluster
	//   uq_finding_by_helm   (cluster_id, helm_release_id)    — one finding per Helm release per cluster
	// PostgreSQL NULL semantics (NULL != NULL) allow multiple findings with null container_image_id or null helm_release_id.
	ClusterID        uuid.UUID       `gorm:"type:uuid;not null;index;uniqueIndex:uq_finding_by_image,priority:1;uniqueIndex:uq_finding_by_helm,priority:1" json:"cluster_id"`
	Cluster          *Cluster        `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	NamespaceName    string          `gorm:"index"                                          json:"namespace_name,omitempty"`
	WorkloadID       *uuid.UUID      `gorm:"type:uuid;index"                                json:"workload_id,omitempty"`
	Workload         *Workload       `gorm:"foreignKey:WorkloadID"                          json:"workload,omitempty"`
	HelmReleaseID    *uuid.UUID      `gorm:"type:uuid;uniqueIndex:uq_finding_by_helm,priority:2" json:"helm_release_id,omitempty"`
	HelmRelease      *HelmRelease    `gorm:"foreignKey:HelmReleaseID"                       json:"helm_release,omitempty"`
	ContainerImageID *uuid.UUID      `gorm:"type:uuid;uniqueIndex:uq_finding_by_image,priority:2" json:"container_image_id,omitempty"`
	ContainerImage   *ContainerImage `gorm:"foreignKey:ContainerImageID"                   json:"container_image,omitempty"`
	Kind             string          `gorm:"not null;index"                                 json:"kind"`
	UpdateType       string          `gorm:"not null"                                       json:"update_type"`
	Severity         string          `gorm:"not null;index"                                 json:"severity"`
	Status           string          `gorm:"not null;default:'open';index"                  json:"status"`
	CurrentVersion   string          `gorm:"not null"                                       json:"current_version"`
	LatestVersion    string          `gorm:"not null"                                       json:"latest_version"`
	Title            string          `gorm:"not null"                                       json:"title"`
	Description      string          `                                                      json:"description,omitempty"`
	ReleaseNotes     string          `                                                      json:"release_notes,omitempty"`
	// column:cves is explicit — GORM's naming strategy maps the CVEs field to
	// "cv_es", which breaks UpsertFinding's ON CONFLICT list and diverges from
	// migrations/001_initial.sql (PostgreSQL), where the column is "cves".
	CVEs              datatypes.JSON `gorm:"column:cves;type:jsonb;default:'[]'"            json:"cves,omitempty"`
	Metadata          datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"metadata,omitempty"`
	FirstDetectedAt   time.Time      `gorm:"not null"                                       json:"first_detected_at"`
	LastObservedAt    time.Time      `gorm:"not null"                                       json:"last_observed_at"`
	StatusChangedAt   *time.Time     `                                                      json:"status_changed_at,omitempty"`
	StatusChangedByID *uuid.UUID     `gorm:"type:uuid"                                      json:"status_changed_by_id,omitempty"`
	PlannedFor        *time.Time     `                                                      json:"planned_for,omitempty"`
	StatusReason      string         `                                                      json:"status_reason,omitempty"`
	ResolvedAt        *time.Time     `                                                      json:"resolved_at,omitempty"`
	CreatedAt         time.Time      `                                                      json:"created_at"`
	UpdatedAt         time.Time      `                                                      json:"updated_at"`
}

// RiskScore holds the computed risk score for an UpdateFinding.
type RiskScore struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	FindingID          uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex"                 json:"finding_id"`
	Finding            *UpdateFinding `gorm:"foreignKey:FindingID"                           json:"finding,omitempty"`
	Score              float64        `gorm:"not null"                                       json:"score"`
	Severity           string         `gorm:"not null"                                       json:"severity"`
	Factors            datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"factors"`
	EnvMultiplier      float64        `gorm:"default:1.0"                                    json:"env_multiplier"`
	ExposureMultiplier float64        `gorm:"default:1.0"                                    json:"exposure_multiplier"`
	ComputedAt         time.Time      `gorm:"not null"                                       json:"computed_at"`
	CreatedAt          time.Time      `                                                      json:"created_at"`
	UpdatedAt          time.Time      `                                                      json:"updated_at"`
}

// Ownership maps workloads to owning teams or persons.
type Ownership struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	WorkloadID  *uuid.UUID `gorm:"type:uuid;index"                                json:"workload_id,omitempty"`
	Workload    *Workload  `gorm:"foreignKey:WorkloadID"                          json:"workload,omitempty"`
	ClusterID   *uuid.UUID `gorm:"type:uuid;index"                                json:"cluster_id,omitempty"`
	Cluster     *Cluster   `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	TeamName    string     `gorm:"not null"                                       json:"team_name"`
	ContactInfo string     `                                                      json:"contact_info,omitempty"`
	CreatedAt   time.Time  `                                                      json:"created_at"`
	UpdatedAt   time.Time  `                                                      json:"updated_at"`
}

// MaintenanceWindow defines when updates/operations are permitted.
type MaintenanceWindow struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ClusterID *uuid.UUID `gorm:"type:uuid;index"                                json:"cluster_id,omitempty"`
	Cluster   *Cluster   `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	Name      string     `gorm:"not null"                                       json:"name"`
	CronExpr  string     `gorm:"not null"                                       json:"cron_expr"`
	Duration  int        `gorm:"not null;default:60"                            json:"duration_minutes"`
	Timezone  string     `gorm:"default:'UTC'"                                  json:"timezone"`
	IsActive  bool       `gorm:"default:true"                                   json:"is_active"`
	CreatedAt time.Time  `                                                      json:"created_at"`
	UpdatedAt time.Time  `                                                      json:"updated_at"`
}

// ActionLog records user actions and system events.
type ActionLog struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	UserID     *uuid.UUID     `gorm:"type:uuid;index"                                json:"user_id,omitempty"`
	User       *User          `gorm:"foreignKey:UserID"                              json:"user,omitempty"`
	Action     string         `gorm:"not null"                                       json:"action"`
	EntityType string         `gorm:"not null"                                       json:"entity_type"`
	EntityID   string         `gorm:"not null"                                       json:"entity_id"`
	Status     string         `gorm:"not null;default:'success';index"              json:"status"`
	Details    datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"details,omitempty"`
	IPAddress  string         `                                                      json:"ip_address,omitempty"`
	UserAgent  string         `                                                      json:"user_agent,omitempty"`
	CreatedAt  time.Time      `gorm:"index"                                          json:"created_at"`
}

// ExceptionRule suppresses a finding for a given scope.
type ExceptionRule struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	ClusterID    *uuid.UUID     `gorm:"type:uuid;index"                                json:"cluster_id,omitempty"`
	Cluster      *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	WorkloadID   *uuid.UUID     `gorm:"type:uuid;index"                                json:"workload_id,omitempty"`
	Workload     *Workload      `gorm:"foreignKey:WorkloadID"                          json:"workload,omitempty"`
	FindingKind  string         `                                                      json:"finding_kind,omitempty"`
	ImagePattern string         `                                                      json:"image_pattern,omitempty"`
	Reason       string         `gorm:"not null"                                       json:"reason"`
	ExpiresAt    *time.Time     `                                                      json:"expires_at,omitempty"`
	CreatedByID  *uuid.UUID     `gorm:"type:uuid"                                      json:"created_by_id,omitempty"`
	Metadata     datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"metadata,omitempty"`
	IsActive     bool           `gorm:"default:true"                                   json:"is_active"`
	CreatedAt    time.Time      `                                                      json:"created_at"`
	UpdatedAt    time.Time      `                                                      json:"updated_at"`
}

// IntegrationAccount stores configuration for external integrations.
type IntegrationAccount struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Name       string         `gorm:"uniqueIndex;not null"                           json:"name"`
	Type       string         `gorm:"not null"                                       json:"type"`
	Config     datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"config,omitempty"`
	SecretRef  string         `                                                      json:"secret_ref,omitempty"`
	IsEnabled  bool           `gorm:"default:true"                                   json:"is_enabled"`
	LastSyncAt *time.Time     `                                                      json:"last_sync_at,omitempty"`
	CreatedAt  time.Time      `                                                      json:"created_at"`
	UpdatedAt  time.Time      `                                                      json:"updated_at"`
}

// User represents an authenticated KubePilot user.
type User struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Email        string     `gorm:"uniqueIndex;not null"                           json:"email"`
	Name         string     `gorm:"not null"                                       json:"name"`
	PasswordHash string     `gorm:"not null"                                       json:"-"`
	Role         string     `gorm:"not null;default:'viewer'"                      json:"role"`
	IsActive     bool       `gorm:"default:true"                                   json:"is_active"`
	LastLoginAt  *time.Time `                                                      json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `                                                      json:"created_at"`
	UpdatedAt    time.Time  `                                                      json:"updated_at"`
}

// UserClusterRole grants a user a role on a specific cluster.
type UserClusterRole struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"  json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_cluster"  json:"user_id"`
	User      *User     `gorm:"foreignKey:UserID"                                json:"user,omitempty"`
	ClusterID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_user_cluster"  json:"cluster_id"`
	Cluster   *Cluster  `gorm:"foreignKey:ClusterID"                             json:"cluster,omitempty"`
	Role      string    `gorm:"not null;default:'viewer'"                        json:"role"`
	CreatedAt time.Time `                                                        json:"created_at"`
	UpdatedAt time.Time `                                                        json:"updated_at"`
}

// AllModels returns a slice of all model instances for AutoMigrate.
func AllModels() []interface{} {
	return []interface{}{
		&Environment{},
		&Cluster{},
		&Namespace{},
		&Node{},
		&NodeMetric{},
		&Workload{},
		&WorkloadSource{},
		&ContainerImage{},
		&ImageRegistry{},
		&ImageTagObservation{},
		&Secret{},
		&HelmRelease{},
		&HelmRepository{},
		&UpdateFinding{},
		&RiskScore{},
		&Ownership{},
		&MaintenanceWindow{},
		&ActionLog{},
		&ExceptionRule{},
		&IntegrationAccount{},
		&User{},
		&UserClusterRole{},
	}
}
