# KubePilot — Risk Scoring Model

## Table of Contents

1. [Overview](#1-overview)
2. [Scoring Formula](#2-scoring-formula)
3. [Severity Thresholds](#3-severity-thresholds)
4. [Factor Reference](#4-factor-reference)
5. [Environment Multiplier](#5-environment-multiplier)
6. [Exposure Multiplier](#6-exposure-multiplier)
7. [Normalization](#7-normalization)
8. [Worked Examples](#8-worked-examples)
9. [Exception Rule Effects](#9-exception-rule-effects)
10. [Special Cases](#10-special-cases)
11. [Score Lifecycle](#11-score-lifecycle)
12. [Implementation Notes](#12-implementation-notes)

---

## 1. Overview

Every UpdateFinding in KubePilot has exactly one RiskScore. The score is a number between 0 and 100 that represents the operational urgency of applying the update, weighted by the current context of the workload, its environment, and its exposure. Higher scores require more immediate attention.

The scoring model is designed around three principles:

1. **Context-sensitivity.** A major version update to a dev-only, headless batch job is very different from a minor update to a production-facing payment API with known CVEs. The score captures this difference.
2. **Auditability.** The full factor breakdown is stored in the `risk_scores.factors` JSONB column. Every score change is timestamped. Operators can see exactly why a workload is scored the way it is.
3. **Tunability.** Environment criticality weights are configurable per environment. This allows organizations to adjust the prod/staging/dev separation to match their own risk posture.

---

## 2. Scoring Formula

```
Score = Normalize( Σ(factor_raw_i × weight_i) × env_multiplier × exposure_multiplier )
```

Where:

- `factor_raw_i` is the raw numeric value for scoring factor `i` (see the factor table below)
- `weight_i` is the fractional weight for factor `i` (all weights sum to exactly 1.0)
- `env_multiplier` is a per-environment scaling factor (0.3 – 1.0)
- `exposure_multiplier` is a scaling factor based on the workload's network exposure (0.8 – 1.2)
- `Normalize()` maps the weighted sum to the 0–100 range (see section 7)

The weighted sum `Σ(factor_raw_i × weight_i)` produces a value in the range 0–20 (the maximum possible raw sum assuming all factors are at their maximum values). This is then scaled by the two multipliers before normalization.

---

## 3. Severity Thresholds

The final normalized score maps to a severity label:

| Score Range | Severity | Color |
|---|---|---|
| 80 – 100 | **Critical** | Red (#e53935) |
| 60 – 79 | **High** | Orange (#fb8c00) |
| 40 – 59 | **Medium** | Yellow (#fdd835) |
| 20 – 39 | **Low** | Blue (#1e88e5) |
| 0 – 19 | **Info** | Grey (#90a4ae) |

Thresholds are inclusive of the lower bound and exclusive of the upper bound, with the exception of 100 being inclusive in the Critical band.

---

## 4. Factor Reference

There are 7 scoring factors. Their weights sum to exactly 1.0 (100%).

---

### Factor 1: Update Type

**Weight: 25%**

Measures the semantic magnitude of the version change. Uses standard semver semantics: a major version bump signals a potentially breaking API change, while a patch update indicates only bug fixes.

| Update Type | Raw Score | Notes |
|---|---|---|
| `patch` | 5 | Bug fix or security patch only |
| `minor` | 15 | New features, backward-compatible |
| `major` | 25 | Potentially breaking changes |
| `unknown` | 10 | Non-semver tag (e.g., date tags, commit SHAs) |

The `is_breaking` flag on UpdateFinding can override the effective raw score to `major`-level (25) regardless of the actual semver bump, for cases where the changelog explicitly documents breaking changes in a minor or patch release.

---

### Factor 2: Update Age

**Weight: 20%**

Measures how long the update has been available without being applied. Older available updates represent a widening security and compatibility gap. Age is calculated from `ImageTagObservation.pushed_at` (or `HelmRelease` chart publication date) to the current time.

| Age Bracket | Raw Score | Notes |
|---|---|---|
| Less than 7 days | 2 | Very recent release; reasonable to wait |
| 7 – 30 days | 8 | Standard update window |
| 30 – 90 days | 15 | Should be addressed in the current sprint |
| More than 90 days | 20 | Significantly overdue |

If `pushed_at` is not available from the registry (some registries omit it), the age is computed from `ImageTagObservation.observed_at` instead, with a note stored in the factor breakdown.

---

### Factor 3: CVSS Maximum Score

**Weight: 20%**

If any CVE is associated with the image tag transition (from a vulnerability scanner integration or a known-CVE database), the maximum CVSS v2 score across all associated CVEs is used. This factor operates on a linear scale: a CVSS 10.0 maps to a raw score of 20.

```
raw_cvss = (cvss_max_v2 / 10.0) × 20
```

| CVSS Max (V2) | Raw Score | Severity Example |
|---|---|---|
| 0.0 (no CVEs) | 0 | No known vulnerabilities |
| 2.5 | 5 | Low CVSS |
| 5.0 | 10 | Medium CVSS |
| 7.5 | 15 | High CVSS |
| 10.0 | 20 | Critical CVSS |

If no vulnerability data is available for the transition, this factor defaults to 0 (not penalized for unknown CVE status). A dedicated integration (e.g., Trivy, Grype, or Snyk) must be configured to populate CVSS data.

---

### Factor 4: Service Criticality

**Weight: 15%**

Measures the business importance of the workload, as declared by the team through ownership metadata or Kubernetes annotations. Set via the `annotations["kubepilot.io/criticality"]` label on the workload, or via the Ownership record.

| Criticality Level | Raw Score | Intended Use |
|---|---|---|
| `critical` | 15 | Revenue-generating, regulatory, or SLA-bound service |
| `high` | 10 | Important internal service; failure is highly visible |
| `medium` | 5 | Standard service; graceful degradation is acceptable |
| `low` | 2 | Batch job, background worker, or internal tooling |

If no criticality annotation is present and no Ownership record specifies it, the default is `medium` (raw score: 5).

---

### Factor 5: Current Health

**Weight: 10%**

Measures the current operational health of the workload. A workload already in a degraded state is at higher risk from an update (less capacity to absorb failures). Health is derived from the workload's `replicas_desired` vs `replicas_ready` ratio and any active pod CrashLoopBackOff events from the K8s Collector.

| Health State | Raw Score | Criteria |
|---|---|---|
| `healthy` | 0 | `replicas_ready == replicas_desired`, no crash loops |
| `warnings` | 5 | `replicas_ready < replicas_desired` but > 50% ready; or recent pod restarts |
| `degraded` | 10 | Less than 50% of desired replicas ready; or active CrashLoopBackOff |

---

### Factor 6: Rollback Possible

**Weight: 5%**

Reflects the operational safety net available if the update goes wrong. Rollback is considered possible if the workload is managed by Helm (helm rollback available) or Argo CD (git revert and sync available). Raw kubectl-managed workloads have uncertain rollback capability.

| Rollback Status | Raw Score | Criteria |
|---|---|---|
| `yes` | 0 | Helm-managed or Argo CD-managed workload |
| `unknown` | 3 | `raw` or `kustomize` source type without detected version history |
| `no` | 5 | Source type is `raw` and no previous revision is detectable |

---

### Factor 7: Maintenance Window

**Weight: 5%**

Reflects whether the update is being assessed during a scheduled maintenance window. Updates evaluated inside a maintenance window carry lower urgency because the team has capacity to apply and monitor them. The scoring engine checks all active MaintenanceWindow records scoped to the workload's cluster and namespace.

| Window Status | Raw Score | Criteria |
|---|---|---|
| `inside` | 0 | Current UTC time falls within an active MaintenanceWindow |
| `outside` | 5 | No active MaintenanceWindow covers this cluster/namespace |

---

### Factor Weight Summary

| Factor | Max Raw | Weight | Max Contribution |
|---|---|---|---|
| Update type | 25 | 25% | 6.25 |
| Update age | 20 | 20% | 4.00 |
| CVSS max | 20 | 20% | 4.00 |
| Service criticality | 15 | 15% | 2.25 |
| Current health | 10 | 10% | 1.00 |
| Rollback possible | 5 | 5% | 0.25 |
| Maintenance window | 5 | 5% | 0.25 |
| **Total** | | **100%** | **18.00** |

The maximum possible weighted sum before multipliers is 18.00.

---

## 5. Environment Multiplier

The environment multiplier scales the entire weighted sum based on the criticality of the environment the workload runs in. This is set on the `Environment.criticality_weight` field.

| Environment Slug | Default Multiplier | Rationale |
|---|---|---|
| `production` | 1.0 | Full weight; production incidents are critical |
| `preprod` | 0.7 | Important but has lower blast radius than production |
| `staging` | 0.5 | Intentionally unstable; updates expected frequently |
| `dev` | 0.3 | Experimental environment; updates are cheap to test |

These are default values. Each environment's `criticality_weight` is configurable by an admin. A custom environment (e.g., `demo`) can be given any weight between 0.0 and 2.0.

---

## 6. Exposure Multiplier

The exposure multiplier scales scores based on whether the workload is reachable from outside the cluster. Externally-exposed services carry higher blast radius risk from an update.

| Exposure Level | Multiplier | Criteria |
|---|---|---|
| External (Ingress) | 1.2 | The workload is the backend for a Kubernetes Ingress resource with a non-internal hostname, or is a NodePort/LoadBalancer Service accessible externally |
| Internal | 1.0 | The workload has a ClusterIP Service but no Ingress or external exposure |
| No service / headless | 0.8 | The workload has no Service selector pointing to it (batch jobs, workers, one-off pods) |

Exposure is determined by the K8s Collector inspecting Service and Ingress resources. For Ingress detection, the hostname is checked against a configurable list of internal domain suffixes (default: `.svc.cluster.local`, `.internal`). Hostnames not matching those suffixes are considered external.

---

## 7. Normalization

The raw weighted sum after multipliers can range from 0 to approximately 21.6 (18.00 × 1.2 max exposure multiplier; environment multiplier maxes at 1.0 by default). To map this to 0–100:

```
normalized_score = min(100, (raw_weighted_sum / MAX_POSSIBLE_SUM) × 100)
```

Where `MAX_POSSIBLE_SUM` is defined as a constant in the scoring engine: `21.6` (corresponding to all factors at maximum value, production environment at 1.0, external exposure at 1.2).

Scores are rounded to one decimal place before storage.

---

## 8. Worked Examples

### Example 1: High-severity production image update with CVEs

**Context:**
- Workload: `payment-api` Deployment in the `payments` namespace
- Environment: `production` (multiplier: 1.0)
- Exposure: External Ingress (multiplier: 1.2)
- Update: image `payment-api:1.4.2` → `payment-api:2.0.0` (major, semver)
- Update age: 45 days
- CVSS max: 7.8 (known CVE in the 1.x series)
- Service criticality: `critical`
- Current health: `healthy`
- Rollback: Helm-managed (`yes`)
- Maintenance window: outside

**Factor calculation:**

| Factor | Raw | Weight | Contribution |
|---|---|---|---|
| Update type (major) | 25 | 0.25 | 6.25 |
| Age (30–90 days) | 15 | 0.20 | 3.00 |
| CVSS max 7.8 → (7.8/10)×20 = 15.6 | 15.6 | 0.20 | 3.12 |
| Service criticality (critical) | 15 | 0.15 | 2.25 |
| Current health (healthy) | 0 | 0.10 | 0.00 |
| Rollback (yes) | 0 | 0.05 | 0.00 |
| Maintenance window (outside) | 5 | 0.05 | 0.25 |
| **Weighted sum** | | | **14.87** |

**Multipliers:**
```
14.87 × 1.0 (prod) × 1.2 (external) = 17.84
```

**Normalization:**
```
(17.84 / 21.6) × 100 = 82.6
```

**Result: Score 82.6 → Severity: Critical**

---

### Example 2: Medium-risk staging Helm chart update

**Context:**
- Workload: `redis-cache` StatefulSet in namespace `cache`, deployed via Helm
- Environment: `staging` (multiplier: 0.5)
- Exposure: ClusterIP only, internal (multiplier: 1.0)
- Update: Helm chart `redis 18.6.1` → `18.7.0` (minor)
- Update age: 12 days
- CVSS max: 0 (no CVEs)
- Service criticality: `medium`
- Current health: `healthy`
- Rollback: Helm-managed (`yes`)
- Maintenance window: outside

**Factor calculation:**

| Factor | Raw | Weight | Contribution |
|---|---|---|---|
| Update type (minor) | 15 | 0.25 | 3.75 |
| Age (7–30 days) | 8 | 0.20 | 1.60 |
| CVSS max (0) | 0 | 0.20 | 0.00 |
| Service criticality (medium) | 5 | 0.15 | 0.75 |
| Current health (healthy) | 0 | 0.10 | 0.00 |
| Rollback (yes) | 0 | 0.05 | 0.00 |
| Maintenance window (outside) | 5 | 0.05 | 0.25 |
| **Weighted sum** | | | **6.35** |

**Multipliers:**
```
6.35 × 0.5 (staging) × 1.0 (internal) = 3.18
```

**Normalization:**
```
(3.18 / 21.6) × 100 = 14.7
```

**Result: Score 14.7 → Severity: Info**

This illustrates how the staging environment multiplier dramatically reduces the urgency of even minor updates in non-production environments.

---

### Example 3: Low-exposure production patch with degraded health

**Context:**
- Workload: `metrics-exporter` DaemonSet in namespace `monitoring`
- Environment: `production` (multiplier: 1.0)
- Exposure: No Service (headless DaemonSet, no port exposed) — multiplier: 0.8
- Update: image patch `node-exporter:1.7.0` → `1.7.1`
- Update age: 3 days
- CVSS max: 0 (no CVEs)
- Service criticality: `low`
- Current health: `degraded` (3/10 nodes have the pod running due to a recent node issue)
- Rollback: Raw kubectl (`unknown`)
- Maintenance window: currently inside a maintenance window

**Factor calculation:**

| Factor | Raw | Weight | Contribution |
|---|---|---|---|
| Update type (patch) | 5 | 0.25 | 1.25 |
| Age (<7 days) | 2 | 0.20 | 0.40 |
| CVSS max (0) | 0 | 0.20 | 0.00 |
| Service criticality (low) | 2 | 0.15 | 0.30 |
| Current health (degraded) | 10 | 0.10 | 1.00 |
| Rollback (unknown) | 3 | 0.05 | 0.15 |
| Maintenance window (inside) | 0 | 0.05 | 0.00 |
| **Weighted sum** | | | **3.10** |

**Multipliers:**
```
3.10 × 1.0 (prod) × 0.8 (no service) = 2.48
```

**Normalization:**
```
(2.48 / 21.6) × 100 = 11.5
```

**Result: Score 11.5 → Severity: Info**

Note how the degraded health (raw score 10, contributing 1.00) is offset by the maintenance window being active and the headless exposure reducing the multiplier to 0.8. The score is still low because the workload is low-criticality with no CVEs and a recent patch.

---

## 9. Exception Rule Effects

ExceptionRule records can modify how scores are applied or presented. The scoring engine evaluates all active (non-expired), scope-matching exception rules after computing the base score.

### Rule Types

**`suppress`**

The finding's score is set to 0 and its severity to `info`. The finding still appears in the findings list but is visually de-emphasized. An "Exception Applied" badge shows the rule name and reason. The finding is excluded from all severity count aggregations.

Typical use: a known false positive, or an image where the organization has accepted the risk permanently.

**`reduce_severity`**

The severity label is downgraded by one level (e.g., Critical → High, High → Medium). The numeric score is not changed. Useful for time-limited exceptions during a freeze period where you want to deprioritize without fully suppressing.

**`accept_risk`**

The finding status is automatically set to `ignored` with a note referencing the exception rule. The score and severity are unchanged. An "Risk Accepted" badge is shown. The finding still counts toward severity totals but is filtered out of default "open" views.

### Scope Matching

Exception rules are matched against a finding by evaluating the rule's `scope_kind` and `scope_selector`:

| Scope Kind | Matching Logic |
|---|---|
| `global` | Matches all findings |
| `cluster` | Matches findings where the target belongs to the specified `cluster_id` |
| `namespace` | Matches findings where the target belongs to the specified `cluster_id` + `namespace` |
| `workload` | Matches findings where `target_id` equals the specified workload UUID |
| `image_pattern` | For image findings only; matches if the image `repository:tag` matches the glob pattern (e.g., `myregistry.io/team/*:latest`) |

### Expiry

Rules with a non-null `expires_at` are automatically excluded from evaluation once the current time exceeds `expires_at`. The scoring engine checks expiry on every score computation. No background job is needed to deactivate rules; they simply stop matching.

Expired rules are retained in the database for audit purposes. The UI shows expired rules with a "Expired" badge on the exception rules management page.

---

## 10. Special Cases

### `latest` Tag Handling

When a container image uses the `latest` tag (or any mutable tag such as `main`, `edge`, `stable`), semantic version comparison is not possible. KubePilot handles this through digest comparison:

1. The Image Watcher stores the `digest` (sha256 manifest hash) from the registry at each observation.
2. On subsequent checks, if the current deployed image's digest differs from the most recently observed digest, a finding is created.
3. The `update_type` is set to `unknown` (raw score: 10) because the change magnitude cannot be determined.
4. The `current_version` and `available_version` in the finding are set to the old and new digest values respectively (truncated to 12 characters for display), not the tag name.

This ensures that teams using mutable tags still receive update notifications, but the scoring reflects the uncertainty inherent in mutable tag management. The Scoring Model documentation for the CVSS factor and the update age factor still apply — age is measured from when the new digest was first observed.

Organizations using `latest` tags in production will consistently see `update_type: unknown` scores. The recommended practice is to pin image tags to immutable versions; the KubePilot overview dashboard shows a "mutable tags in production" metric to help track progress.

### Digest-only Image References

If a container spec uses a digest-only reference (no tag at all, e.g., `nginx@sha256:abc123`), the Image Watcher cannot poll for updates because there is no tag to compare against. These workloads are excluded from image update tracking. A dedicated "no tag" warning is shown on the workload detail page.

### Helm Chart `appVersion` vs. `version`

Helm findings track the chart `version` (e.g., `4.9.1`), not the `appVersion` string. The chart version is semver-parseable and comparable. `appVersion` is displayed informatively in the UI but does not drive finding creation or scoring.

### New Clusters (Cold Start)

When a new cluster is first registered and the K8s Collector completes its initial list, all observed images are new. The Image Watcher will check every observed image against its registry. If newer tags already exist, findings are created immediately with age computed from when those tags were first published (using `pushed_at` from the registry). This means a new cluster registration can immediately surface a backlog of overdue updates, which is intentional.

---

## 11. Score Lifecycle

Scores are recomputed by the Scoring Engine in the following situations:

1. **New finding created** — immediately on finding creation.
2. **Image tag observation updated** — when a new tag is seen, the age factor changes for any related finding.
3. **Workload health change** — when the K8s Collector observes a change in replica count or pod conditions.
4. **Maintenance window transition** — the Job Scheduler triggers score recomputation for all findings scoped to a cluster/namespace when a maintenance window starts or ends.
5. **Exception rule created or expired** — triggers recomputation for all findings in scope.
6. **Environment criticality_weight changed** — triggers recomputation for all findings across all clusters in that environment.
7. **Periodic refresh** — the Job Scheduler recomputes all scores every 6 hours to catch age bracket transitions (e.g., an update crossing the 30-day threshold).

Each recomputation writes a new `risk_scores` record (replacing the previous one via `ON CONFLICT DO UPDATE`) and publishes a `score.updated` event to Redis Pub/Sub.

---

## 12. Implementation Notes

### Go Package: `internal/scoring`

The scoring engine is implemented as a pure function `Compute(ctx context.Context, fc FindingContext) (RiskScore, error)`. The `FindingContext` struct bundles all data needed for scoring:

```go
type FindingContext struct {
    Finding           db.UpdateFinding
    TargetWorkload    *db.Workload          // nil for HelmRelease/Node targets
    TargetHelmRelease *db.HelmRelease       // nil for Workload/Node targets
    Environment       db.Environment
    ExposureLevel     ExposureLevel         // computed by caller from K8s resources
    Ownership         *db.Ownership         // nil if not set
    CVSSMax           float64               // 0 if no CVE data
    ActiveExceptions  []db.ExceptionRule
    InMaintenanceWin  bool
}
```

This design keeps the scoring function free of database calls and easy to unit-test. The caller (worker or handler) is responsible for fetching all required context and passing it in.

### Factor Weights as Constants

Weights are defined as typed constants in `internal/scoring/factors.go`:

```go
const (
    WeightUpdateType       = 0.25
    WeightAge              = 0.20
    WeightCVSSMax          = 0.20
    WeightServiceCritical  = 0.15
    WeightCurrentHealth    = 0.10
    WeightRollback         = 0.05
    WeightMaintenanceWin   = 0.05

    MaxPossibleSum = 21.6
)
```

If you modify any weight, ensure all weights still sum to 1.0 and update `MaxPossibleSum` accordingly. A compile-time assertion checks this:

```go
const _ = 1 / uint(WeightUpdateType + WeightAge + WeightCVSSMax +
    WeightServiceCritical + WeightCurrentHealth + WeightRollback +
    WeightMaintenanceWin == 1.0)
```

### Factor Breakdown in `risk_scores.factors`

The `factors` JSONB column stores the complete breakdown for UI display and debugging. Structure:

```json
{
  "update_type":         { "value": "minor", "raw": 15, "weight": 0.25, "contribution": 3.75 },
  "age":                 { "value": "7-30d",  "raw": 8,  "weight": 0.20, "contribution": 1.60 },
  "cvss_max":            { "value": 0.0,      "raw": 0,  "weight": 0.20, "contribution": 0.00 },
  "service_criticality": { "value": "high",   "raw": 10, "weight": 0.15, "contribution": 1.50 },
  "current_health":      { "value": "healthy","raw": 0,  "weight": 0.10, "contribution": 0.00 },
  "rollback_possible":   { "value": "yes",    "raw": 0,  "weight": 0.05, "contribution": 0.00 },
  "maintenance_window":  { "value": "outside","raw": 5,  "weight": 0.05, "contribution": 0.25 },
  "weighted_sum":        7.10,
  "env_multiplier":      1.0,
  "exposure_multiplier": 1.2,
  "pre_norm":            8.52,
  "max_possible_sum":    21.6,
  "normalized_score":    39.4,
  "exception_applied":   null
}
```

When an exception rule is applied, `exception_applied` contains:

```json
{
  "rule_id": "uuid",
  "rule_name": "Freeze window Q1",
  "rule_type": "reduce_severity",
  "reason": "Regulatory freeze period Jan–Mar 2024"
}
```
