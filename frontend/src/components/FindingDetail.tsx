import { ExternalLink, Copy, Clock, Server, Package } from 'lucide-react'
import clsx from 'clsx'
import { SeverityBadge } from './SeverityBadge'
import { FindingStatusMenu } from './FindingStatusMenu'
import type { UpdateFinding } from '../types'
import { formatAge, formatRelative, scoreToColor, scoreToBg, buildHeadlampURL, updateTypeLabel } from '../utils/formatting'

const UPDATE_TYPE_STYLES: Record<string, string> = {
  major: 'bg-red-950/60 text-red-400 border border-red-900/50',
  minor: 'bg-yellow-950/60 text-yellow-400 border border-yellow-900/50',
  patch: 'bg-green-950/60 text-green-400 border border-green-900/50',
  unknown: 'bg-slate-800 text-slate-400 border border-slate-700',
}

interface Props {
  finding: UpdateFinding
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

export function FindingDetail({ finding }: Props) {
  const headlampBase = import.meta.env.VITE_HEADLAMP_URL || 'http://localhost:4466'
  const headlampURL = finding.cluster_name && finding.namespace_name
    ? buildHeadlampURL(
        headlampBase,
        finding.cluster_name,
        finding.namespace_name,
        finding.target_kind,
        finding.workload_name || finding.target_id
      )
    : null

  function copyToClipboard(text: string) {
    navigator.clipboard.writeText(text).catch(() => {})
  }

  const isHelm = finding.target_kind === 'HelmRelease'
  const helmCommand = isHelm
    ? `helm upgrade ${finding.workload_name} --version ${finding.latest_version} -n ${finding.namespace_name}`
    : null

  return (
    <div className="p-4 space-y-5">
      {/* Header */}
      <div className="space-y-2">
        <div className="flex items-start gap-2 flex-wrap">
          {finding.risk_score && <SeverityBadge severity={finding.risk_score.severity} size="md" />}
          <span
            className={clsx('badge text-xs font-medium', UPDATE_TYPE_STYLES[finding.update_type])}
          >
            {updateTypeLabel(finding.update_type)}
          </span>
          {finding.is_breaking && (
            <span className="badge bg-red-950/80 text-red-400 border border-red-900/60 text-xs">
              BREAKING
            </span>
          )}
        </div>
        <h3 className="text-base font-semibold text-slate-100">
          {finding.workload_name || finding.target_id}
        </h3>
        <p className="text-xs text-slate-500 font-mono">{finding.target_kind}</p>
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
      {finding.risk_score && (
        <div className="panel p-3 space-y-3">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Risk Score</div>
          <ScoreBar score={finding.risk_score.score} />
          {Object.keys(finding.risk_score.factors ?? {}).length > 0 && (
            <table className="w-full text-xs">
              <tbody>
                {Object.entries(finding.risk_score.factors ?? {}).map(([factor, value]) => (
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
            <span className="text-slate-300">{formatRelative(finding.last_confirmed_at)}</span>
          </div>
        </div>
      </div>

      {/* Actions */}
      <div className="space-y-2">
        {finding.changelog_url && (
          <a
            href={finding.changelog_url}
            target="_blank"
            rel="noopener noreferrer"
            className="btn btn-secondary w-full justify-center"
          >
            <ExternalLink size={14} />
            View Changelog
          </a>
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
        {helmCommand && (
          <div className="panel p-3 space-y-2">
            <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Upgrade Command</div>
            <div className="flex items-start gap-2">
              <code className="flex-1 text-xs font-mono text-green-400 bg-surface-elevated p-2 rounded break-all">
                {helmCommand}
              </code>
              <button
                className="btn btn-secondary flex-shrink-0 p-1.5"
                onClick={() => copyToClipboard(helmCommand)}
                title="Copy command"
              >
                <Copy size={14} />
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
