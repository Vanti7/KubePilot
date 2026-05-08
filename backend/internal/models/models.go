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
	ActionLogCreate        = "create"
	ActionLogUpdate        = "update"
	ActionLogDelete        = "delete"
	ActionLogUpdateStatus  = "update_status"
	ActionLogResolve       = "resolve"
	ActionLogIgnore        = "ignore"
	ActionLogSync          = "sync"
)

// User roles
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

// Environment represents a deployment environment (prod, staging, dev, etc.).
type Environment struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name              string    `gorm:"uniqueIndex;not null"                           json:"name"`
	Slug              string    `gorm:"uniqueIndex;not null"                           json:"slug"`
	CriticalityWeight float64   `gorm:"default:1.0"                                    json:"criticality_weight"`
	Color             string    `gorm:"default:'#6B7280'"                              json:"color"`
	CreatedAt         time.Time `                                                      json:"created_at"`
	UpdatedAt         time.Time `                                                      json:"updated_at"`
}

// Cluster represents a Kubernetes cluster registered in KubePilot.
type Cluster struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	EnvironmentID   *uuid.UUID     `gorm:"type:uuid;index"                                json:"environment_id,omitempty"`
	Environment     *Environment   `gorm:"foreignKey:EnvironmentID"                       json:"environment,omitempty"`
	Name            string         `gorm:"uniqueIndex;not null"                           json:"name"`
	Slug            string         `gorm:"uniqueIndex;not null"                           json:"slug"`
	Provider        string         `gorm:"default:'unknown'"                              json:"provider"`
	Region          string         `                                                      json:"region"`
	K8sVersion      string         `                                                      json:"k8s_version"`
	Status          string         `gorm:"default:'unknown'"                              json:"status"`
	LastSeenAt      *time.Time     `                                                      json:"last_seen_at,omitempty"`
	KubeconfigRef   string         `gorm:"column:kubeconfig_ref"                          json:"kubeconfig_ref,omitempty"`
	APIEndpoint     string         `                                                      json:"api_endpoint,omitempty"`
	TLSInsecure     bool           `gorm:"default:false"                                  json:"tls_insecure"`
	Annotations     datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"annotations,omitempty"`
	Labels          datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"labels,omitempty"`
	CreatedAt       time.Time      `                                                      json:"created_at"`
	UpdatedAt       time.Time      `                                                      json:"updated_at"`
}

// Namespace represents a Kubernetes namespace.
type Namespace struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
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
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClusterID      uuid.UUID      `gorm:"type:uuid;not null;index"                       json:"cluster_id"`
	Cluster        *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	Name           string         `gorm:"not null"                                       json:"name"`
	Role           string         `gorm:"default:'worker'"                               json:"role"`
	Status         string         `gorm:"default:'unknown'"                              json:"status"`
	K8sVersion     string         `                                                      json:"k8s_version"`
	OSImage        string         `                                                      json:"os_image"`
	KernelVersion  string         `                                                      json:"kernel_version"`
	ContainerRuntime string       `                                                      json:"container_runtime"`
	Arch           string         `                                                      json:"arch"`
	CapacityCPU    string         `                                                      json:"capacity_cpu"`
	CapacityMemory string         `                                                      json:"capacity_memory"`
	AllocatableCPU string         `                                                      json:"allocatable_cpu"`
	AllocatableMemory string      `                                                      json:"allocatable_memory"`
	Labels         datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"labels,omitempty"`
	Taints         datatypes.JSON `gorm:"type:jsonb;default:'[]'"                        json:"taints,omitempty"`
	Conditions     datatypes.JSON `gorm:"type:jsonb;default:'[]'"                        json:"conditions,omitempty"`
	CreatedAt      time.Time      `                                                      json:"created_at"`
	UpdatedAt      time.Time      `                                                      json:"updated_at"`
}

// Workload represents a Kubernetes workload (Deployment, DaemonSet, StatefulSet).
type Workload struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClusterID       uuid.UUID      `gorm:"type:uuid;not null;index"                       json:"cluster_id"`
	Cluster         *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	NamespaceID     *uuid.UUID     `gorm:"type:uuid;index"                                json:"namespace_id,omitempty"`
	Namespace       *Namespace     `gorm:"foreignKey:NamespaceID"                         json:"namespace,omitempty"`
	Name            string         `gorm:"not null"                                       json:"name"`
	NamespaceName   string         `gorm:"not null;index"                                 json:"namespace_name"`
	Kind            string         `gorm:"not null"                                       json:"kind"`
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
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkloadID     uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_workload_source" json:"workload_id"`
	Workload       *Workload  `gorm:"foreignKey:WorkloadID"                              json:"workload,omitempty"`
	HelmReleaseID  *uuid.UUID `gorm:"type:uuid;index"                                    json:"helm_release_id,omitempty"`
	HelmRelease    *HelmRelease `gorm:"foreignKey:HelmReleaseID"                         json:"helm_release,omitempty"`
	SourceType     string     `gorm:"default:'unknown'"                                  json:"source_type"`
	CreatedAt      time.Time  `                                                          json:"created_at"`
	UpdatedAt      time.Time  `                                                          json:"updated_at"`
}

// ContainerImage represents a container image used by a workload.
type ContainerImage struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WorkloadID      uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"workload_id"`
	Workload        *Workload  `gorm:"foreignKey:WorkloadID"                          json:"workload,omitempty"`
	ContainerName   string     `gorm:"not null"                                       json:"container_name"`
	Image           string     `gorm:"not null"                                       json:"image"`
	Registry        string     `gorm:"not null"                                       json:"registry"`
	Repository      string     `gorm:"not null"                                       json:"repository"`
	Tag             string     `gorm:"not null"                                       json:"tag"`
	Digest          string     `                                                      json:"digest,omitempty"`
	IsInitContainer bool       `gorm:"default:false"                                  json:"is_init_container"`
	RegistryID      *uuid.UUID `gorm:"type:uuid;index"                                json:"registry_id,omitempty"`
	ImageRegistry   *ImageRegistry `gorm:"foreignKey:RegistryID"                      json:"image_registry,omitempty"`
	CreatedAt       time.Time  `                                                      json:"created_at"`
	UpdatedAt       time.Time  `                                                      json:"updated_at"`
}

// ImageRegistry stores registry configuration and credentials.
type ImageRegistry struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name           string         `gorm:"uniqueIndex;not null"                           json:"name"`
	Host           string         `gorm:"uniqueIndex;not null"                           json:"host"`
	Type           string         `gorm:"default:'generic'"                              json:"type"`
	CredentialsRef string         `                                                      json:"credentials_ref,omitempty"`
	AuthConfig     datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"auth_config,omitempty"`
	RateLimitRPM   int            `gorm:"default:60"                                     json:"rate_limit_rpm"`
	CreatedAt      time.Time      `                                                      json:"created_at"`
	UpdatedAt      time.Time      `                                                      json:"updated_at"`
}

// ImageTagObservation records a tag observed in a registry at a point in time.
type ImageTagObservation struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ImageID     uuid.UUID  `gorm:"type:uuid;not null;index"                       json:"image_id"`
	Image       *ContainerImage `gorm:"foreignKey:ImageID"                        json:"image,omitempty"`
	Tag         string     `gorm:"not null"                                       json:"tag"`
	Digest      string     `                                                      json:"digest,omitempty"`
	PublishedAt *time.Time `                                                      json:"published_at,omitempty"`
	ObservedAt  time.Time  `gorm:"not null"                                       json:"observed_at"`
	IsLatest    bool       `gorm:"default:false"                                  json:"is_latest"`
}

// HelmRelease represents a Helm release deployed in a cluster.
type HelmRelease struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClusterID      uuid.UUID      `gorm:"type:uuid;not null;index"                       json:"cluster_id"`
	Cluster        *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	NamespaceName  string         `gorm:"not null;index"                                 json:"namespace_name"`
	Name           string         `gorm:"not null"                                       json:"name"`
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
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClusterID         uuid.UUID      `gorm:"type:uuid;not null;index"                       json:"cluster_id"`
	Cluster           *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	NamespaceName     string         `gorm:"index"                                          json:"namespace_name,omitempty"`
	WorkloadID        *uuid.UUID     `gorm:"type:uuid;index"                                json:"workload_id,omitempty"`
	Workload          *Workload      `gorm:"foreignKey:WorkloadID"                          json:"workload,omitempty"`
	HelmReleaseID     *uuid.UUID     `gorm:"type:uuid;index"                                json:"helm_release_id,omitempty"`
	HelmRelease       *HelmRelease   `gorm:"foreignKey:HelmReleaseID"                       json:"helm_release,omitempty"`
	ContainerImageID  *uuid.UUID     `gorm:"type:uuid;index"                                json:"container_image_id,omitempty"`
	ContainerImage    *ContainerImage `gorm:"foreignKey:ContainerImageID"                   json:"container_image,omitempty"`
	Kind              string         `gorm:"not null;index"                                 json:"kind"`
	UpdateType        string         `gorm:"not null"                                       json:"update_type"`
	Severity          string         `gorm:"not null;index"                                 json:"severity"`
	Status            string         `gorm:"not null;default:'open';index"                  json:"status"`
	CurrentVersion    string         `gorm:"not null"                                       json:"current_version"`
	LatestVersion     string         `gorm:"not null"                                       json:"latest_version"`
	Title             string         `gorm:"not null"                                       json:"title"`
	Description       string         `                                                      json:"description,omitempty"`
	ReleaseNotes      string         `                                                      json:"release_notes,omitempty"`
	CVEs              datatypes.JSON `gorm:"type:jsonb;default:'[]'"                        json:"cves,omitempty"`
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
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	FindingID    uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex"                 json:"finding_id"`
	Finding      *UpdateFinding `gorm:"foreignKey:FindingID"                           json:"finding,omitempty"`
	Score        float64        `gorm:"not null"                                       json:"score"`
	Severity     string         `gorm:"not null"                                       json:"severity"`
	Factors      datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"factors"`
	EnvMultiplier float64       `gorm:"default:1.0"                                    json:"env_multiplier"`
	ExposureMultiplier float64  `gorm:"default:1.0"                                    json:"exposure_multiplier"`
	ComputedAt   time.Time      `gorm:"not null"                                       json:"computed_at"`
	CreatedAt    time.Time      `                                                      json:"created_at"`
	UpdatedAt    time.Time      `                                                      json:"updated_at"`
}

// Ownership maps workloads to owning teams or persons.
type Ownership struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
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
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClusterID   *uuid.UUID `gorm:"type:uuid;index"                                json:"cluster_id,omitempty"`
	Cluster     *Cluster   `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	Name        string     `gorm:"not null"                                       json:"name"`
	CronExpr    string     `gorm:"not null"                                       json:"cron_expr"`
	Duration    int        `gorm:"not null;default:60"                            json:"duration_minutes"`
	Timezone    string     `gorm:"default:'UTC'"                                  json:"timezone"`
	IsActive    bool       `gorm:"default:true"                                   json:"is_active"`
	CreatedAt   time.Time  `                                                      json:"created_at"`
	UpdatedAt   time.Time  `                                                      json:"updated_at"`
}

// ActionLog records user actions and system events.
type ActionLog struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID     *uuid.UUID     `gorm:"type:uuid;index"                                json:"user_id,omitempty"`
	User       *User          `gorm:"foreignKey:UserID"                              json:"user,omitempty"`
	Action     string         `gorm:"not null"                                       json:"action"`
	EntityType string         `gorm:"not null"                                       json:"entity_type"`
	EntityID   string         `gorm:"not null"                                       json:"entity_id"`
	Details    datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"details,omitempty"`
	IPAddress  string         `                                                      json:"ip_address,omitempty"`
	UserAgent  string         `                                                      json:"user_agent,omitempty"`
	CreatedAt  time.Time      `gorm:"index"                                          json:"created_at"`
}

// ExceptionRule suppresses a finding for a given scope.
type ExceptionRule struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ClusterID     *uuid.UUID     `gorm:"type:uuid;index"                                json:"cluster_id,omitempty"`
	Cluster       *Cluster       `gorm:"foreignKey:ClusterID"                           json:"cluster,omitempty"`
	WorkloadID    *uuid.UUID     `gorm:"type:uuid;index"                                json:"workload_id,omitempty"`
	Workload      *Workload      `gorm:"foreignKey:WorkloadID"                          json:"workload,omitempty"`
	FindingKind   string         `                                                      json:"finding_kind,omitempty"`
	ImagePattern  string         `                                                      json:"image_pattern,omitempty"`
	Reason        string         `gorm:"not null"                                       json:"reason"`
	ExpiresAt     *time.Time     `                                                      json:"expires_at,omitempty"`
	CreatedByID   *uuid.UUID     `gorm:"type:uuid"                                      json:"created_by_id,omitempty"`
	Metadata      datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"metadata,omitempty"`
	IsActive      bool           `gorm:"default:true"                                   json:"is_active"`
	CreatedAt     time.Time      `                                                      json:"created_at"`
	UpdatedAt     time.Time      `                                                      json:"updated_at"`
}

// IntegrationAccount stores configuration for external integrations.
type IntegrationAccount struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string         `gorm:"uniqueIndex;not null"                           json:"name"`
	Type        string         `gorm:"not null"                                       json:"type"`
	Config      datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"config,omitempty"`
	SecretRef   string         `                                                      json:"secret_ref,omitempty"`
	IsEnabled   bool           `gorm:"default:true"                                   json:"is_enabled"`
	LastSyncAt  *time.Time     `                                                      json:"last_sync_at,omitempty"`
	CreatedAt   time.Time      `                                                      json:"created_at"`
	UpdatedAt   time.Time      `                                                      json:"updated_at"`
}

// User represents an authenticated KubePilot user.
type User struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
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
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"  json:"id"`
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
		&Workload{},
		&WorkloadSource{},
		&ContainerImage{},
		&ImageRegistry{},
		&ImageTagObservation{},
		&HelmRelease{},
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
