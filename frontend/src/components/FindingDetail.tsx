import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ExternalLink, Wrench, Clock, Server, Package } from 'lucide-react'
import clsx from 'clsx'
import { SeverityBadge } from './SeverityBadge'
import { FindingStatusMenu } from './FindingStatusMenu'
import { remediateFinding } from '../api/client'
import type { UpdateFinding } from '../types'
import { formatAge, formatRelative, scoreToColor, scoreToBg, buildHeadlampURL, updateTypeLabel } from '../utils/formatting'

// Statuses RemediateFinding accepts server-side (store.IsActiveFindingStatus) —
// mirrored here so the button doesn't invite a click that the API would 400.
const ACTIONABLE_STATUSES = new Set(['open', 'planned', 'approved'])

// A real fix (image bump / Helm upgrade) only shows up as "resolved" once the
// collector re-syncs the live state AND the watcher re-checks it — the
// backend retries this itself for ~21s (3s/6s/12s). Re-invalidate a bit
// longer than that window to reliably catch the resolution.
function refreshAfterFix(qc: ReturnType<typeof useQueryClient>) {
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['findings'] })
    qc.invalidateQueries({ queryKey: ['workloads'] })
    qc.invalidateQueries({ queryKey: ['helm'] })
  }
  invalidate()
  ;[2000, 5000, 10000, 20000].forEach((ms) => setTimeout(invalidate, ms))
}

const UPDATE_TYPE_STYLES: Record<string, string> = {
  major: 'bg-red-950/60 text-red-400 border border-red-900/50',
  minor: 'bg-yellow-950/60 text-yellow-400 border border-yellow-900/50',
  patch: 'bg-green-950/60 text-green-400 border border-green-900/50',
  unknown: 'bg-slate-800 text-slate-400 border border-slate-700',
}

interface Props {
  finding: UpdateFinding
  canWrite?: boolean
}

function ScoreBar({ score }: { score: number }) {
  return (
    <div className="flex items-center gap-2">
      <div className="flex-1 h-2 bg-surface-elevated rounded-full overflow-hidden">
        <div
          className={clsx('h-full rounded-full transition-all', scoreToBg(score))}
          style={{ width: `${score}%` }}
        />
      </div>
      <span className={clsx('text-sm font-mono font-semibold w-8 text-right', scoreToColor(score))}>
        {score}
      </span>
    </div>
  )
}

export function FindingDetail({ finding, canWrite }: Props) {
  const qc = useQueryClient()
  const [busy, setBusy] = useState(false)
  const [fixError, setFixError] = useState('')
  const [applied, setApplied] = useState(false)

  const headlampBase = import.meta.env.VITE_HEADLAMP_URL || 'http://localhost:4466'
  // Helm findings carry no workload, so there is no Kubernetes resource to link to.
  const headlampURL =
    finding.cluster_name && finding.namespace_name && finding.workload_kind && finding.workload_name
      ? buildHeadlampURL(
          headlampBase,
          finding.cluster_name,
          finding.namespace_name,
          finding.workload_kind,
          finding.workload_name
        )
      : null

  const isHelm = finding.kind === 'helm'
  const isImage = finding.kind === 'image'
  const canFixHelm = isHelm && !!finding.helm_release_name
  const canFixImage = isImage && !!finding.container_name && !!finding.image_registry && !!finding.image_repository
  const canFix = canWrite && ACTIONABLE_STATUSES.has(finding.status) && (canFixHelm || canFixImage)

  async function handleFix() {
    const target = finding.workload_name || finding.helm_release_name || finding.title
    if (!confirm(`Update ${target} to ${finding.latest_version} now? This changes the live cluster.`)) {
      return
    }
    setBusy(true)
    setFixError('')
    try {
      await remediateFinding(finding.id)
      setApplied(true)
      refreshAfterFix(qc)
    } catch (e: any) {
      setFixError(e?.response?.data?.error || e.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="p-4 space-y-5">
      {/* Header */}
      <div className="space-y-2">
        <div className="flex items-start gap-2 flex-wrap">
          <SeverityBadge severity={finding.score_severity || finding.severity} size="md" />
          <span
            className={clsx('badge text-xs font-medium', UPDATE_TYPE_STYLES[finding.update_type])}
          >
            {updateTypeLabel(finding.update_type)}
          </span>
        </div>
        <h3 className="text-base font-semibold text-slate-100">
          {finding.workload_name || finding.helm_release_name || finding.title}
        </h3>
        <p className="text-xs text-slate-500 font-mono">
          {finding.workload_kind || (isHelm ? 'HelmRelease' : finding.kind)}
        </p>
      </div>

      {/* Version */}
      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Version</div>
        <div className="flex items-center gap-2 font-mono text-sm">
          <span className="text-slate-400">{finding.current_version}</span>
          <span className="text-slate-600">→</span>
          <span className="text-green-400 font-semibold">{finding.latest_version}</span>
        </div>
      </div>

      {/* Risk Score */}
      {finding.score != null && (
        <div className="panel p-3 space-y-3">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Risk Score</div>
          <ScoreBar score={finding.score} />
          {Object.keys(finding.score_factors ?? {}).length > 0 && (
            <table className="w-full text-xs">
              <tbody>
                {Object.entries(finding.score_factors ?? {}).map(([factor, value]) => (
                  <tr key={factor} className="border-t border-surface-border/50">
                    <td className="py-1.5 text-slate-400 capitalize">{factor.replace(/_/g, ' ')}</td>
                    <td className="py-1.5 text-right">
                      <div className="inline-flex items-center gap-1.5">
                        <div className="w-16 h-1.5 bg-surface-elevated rounded-full overflow-hidden">
                          <div
                            className={clsx('h-full rounded-full', scoreToBg(value * 100))}
                            style={{ width: `${Math.min(value * 100, 100)}%` }}
                          />
                        </div>
                        <span className={clsx('font-mono', scoreToColor(value * 100))}>
                          {(value * 100).toFixed(0)}
                        </span>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}

      {/* Status */}
      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Status</div>
        <FindingStatusMenu findingId={finding.id} currentStatus={finding.status} />
      </div>

      {/* Location */}
      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Location</div>
        <div className="space-y-1.5 text-xs">
          {finding.cluster_name && (
            <div className="flex items-center gap-2 text-slate-300">
              <Server size={12} className="text-slate-500 flex-shrink-0" />
              <span className="text-slate-400">Cluster</span>
              <span className="font-mono">{finding.cluster_name}</span>
            </div>
          )}
          {finding.namespace_name && (
            <div className="flex items-center gap-2 text-slate-300">
              <Package size={12} className="text-slate-500 flex-shrink-0" />
              <span className="text-slate-400">Namespace</span>
              <span className="font-mono">{finding.namespace_name}</span>
            </div>
          )}
        </div>
      </div>

      {/* Timestamps */}
      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Timeline</div>
        <div className="space-y-1.5 text-xs">
          <div className="flex items-center gap-2">
            <Clock size={12} className="text-slate-500 flex-shrink-0" />
            <span className="text-slate-400">First detected</span>
            <span className="text-slate-300">{formatRelative(finding.first_detected_at)}</span>
            <span className="text-slate-600">({formatAge(finding.first_detected_at)} ago)</span>
          </div>
          <div className="flex items-center gap-2">
            <Clock size={12} className="text-slate-500 flex-shrink-0" />
            <span className="text-slate-400">Last confirmed</span>
            <span className="text-slate-300">{formatRelative(finding.last_observed_at)}</span>
          </div>
        </div>
      </div>

      {/* Actions */}
      <div className="space-y-2">
        {canFix && (
          <button
            className="btn btn-primary w-full justify-center"
            disabled={busy}
            onClick={handleFix}
          >
            <Wrench size={14} className={busy ? 'animate-pulse' : ''} />
            {busy ? 'Applying…' : `Fix now → ${finding.latest_version}`}
          </button>
        )}
        {applied && (
          <div className="px-3 py-2 rounded bg-green-950/60 border border-green-900/50 text-green-400 text-xs">
            Applied — the finding will resolve automatically within a few seconds once the fix is confirmed.
          </div>
        )}
        {fixError && (
          <div className="px-3 py-2 rounded bg-red-950/60 border border-red-900/50 text-red-400 text-xs">{fixError}</div>
        )}
        {headlampURL && (
          <a
            href={headlampURL}
            target="_blank"
            rel="noopener noreferrer"
            className="btn btn-secondary w-full justify-center"
          >
            <ExternalLink size={14} />
            Open in Headlamp
          </a>
        )}
      </div>
    </div>
  )
}
