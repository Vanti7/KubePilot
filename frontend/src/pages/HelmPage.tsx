import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ExternalLink, ArrowUpCircle, History } from 'lucide-react'
import clsx from 'clsx'
import { getHelmReleases, getHelmRelease, getFindings, upgradeHelmRelease, rollbackHelmRelease } from '../api/client'
import { useClusters } from '../hooks/useClusters'
import { useAuth } from '../contexts/AuthContext'
import { DataTable, Column } from '../components/DataTable'
import { SlideOver } from '../components/SlideOver'
import type { HelmRelease } from '../types'
import { formatAge } from '../utils/formatting'

const HELM_STATUS_STYLES: Record<string, string> = {
  deployed: 'text-green-400',
  failed: 'text-red-400',
  pending: 'text-yellow-400',
  superseded: 'text-slate-500',
  uninstalled: 'text-slate-600',
}

// Helm releases are only re-polled by the backend collector every 60s (no
// live Watch), and upgrade/rollback trigger an immediate background pass —
// but it still runs after the response comes back, so invalidate a couple
// more times over the next few seconds to actually catch it (same fix
// applied to workloads' scale/restart).
function refreshHelmSoon(qc: ReturnType<typeof useQueryClient>, releaseId: string) {
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['helm'] })
    qc.invalidateQueries({ queryKey: ['helm-release', releaseId] })
  }
  invalidate()
  setTimeout(invalidate, 1500)
  setTimeout(invalidate, 4000)
}

function HelmDetail({ releaseId, canWrite }: { releaseId: string; canWrite: boolean }) {
  const qc = useQueryClient()
  const [busy, setBusy] = useState(false)
  const [chartVersion, setChartVersion] = useState('')
  const [valuesText, setValuesText] = useState('')
  const [valuesError, setValuesError] = useState('')
  const [rollbackRevision, setRollbackRevision] = useState('')
  const [error, setError] = useState('')

  const { data: release } = useQuery({
    queryKey: ['helm-release', releaseId],
    queryFn: () => getHelmRelease(releaseId),
  })

  // Cross-reference the finding this release already produces (if any) —
  // "Findings Helm réels" already computes the latest available version,
  // no need to duplicate that logic here.
  const { data: findings } = useQuery({
    queryKey: ['helm-findings', release?.cluster_id],
    queryFn: () => getFindings({ cluster_id: release?.cluster_id, kind: 'helm', limit: 500 }),
    enabled: !!release?.cluster_id,
  })
  const finding = findings?.data.find((f) => f.helm_release_id === releaseId)

  if (!release) return null

  const valuesPlaceholder = JSON.stringify(release.values ?? {}, null, 2)

  async function handleUpgrade() {
    setError('')
    setValuesError('')
    let values: Record<string, any> | undefined
    if (valuesText.trim()) {
      try {
        values = JSON.parse(valuesText)
      } catch (e: any) {
        setValuesError('Invalid JSON: ' + e.message)
        return
      }
    }
    setBusy(true)
    try {
      await upgradeHelmRelease(releaseId, {
        chart_version: chartVersion.trim() || undefined,
        values,
      })
      refreshHelmSoon(qc, releaseId)
    } catch (e: any) {
      setError(e?.response?.data?.error || e.message)
    } finally {
      setBusy(false)
    }
  }

  async function handleRollback() {
    const revision = Number(rollbackRevision)
    if (!Number.isInteger(revision) || revision < 1) {
      setError('Revision must be a whole number >= 1')
      return
    }
    if (!confirm(`Roll back ${release!.name} to revision ${revision}?`)) return
    setError('')
    setBusy(true)
    try {
      await rollbackHelmRelease(releaseId, revision)
      refreshHelmSoon(qc, releaseId)
    } catch (e: any) {
      setError(e?.response?.data?.error || e.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="p-4 space-y-4">
      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Release</div>
        <div className="space-y-1.5 text-xs">
          <div className="flex gap-2">
            <span className="text-slate-500 w-28">Release name</span>
            <span className="text-slate-200 font-mono">{release.name}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-28">Chart</span>
            <span className="text-slate-200 font-mono">{release.chart_name}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-28">Namespace</span>
            <span className="text-slate-300 font-mono">{release.namespace_name}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-28">Cluster</span>
            <span className="text-slate-300 font-mono">{release.cluster?.name}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-28">Status</span>
            <span className={clsx('font-medium capitalize', HELM_STATUS_STYLES[release.status] || 'text-slate-400')}>
              {release.status}
            </span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-28">Revision</span>
            <span className="text-slate-300 font-mono">{release.revision}</span>
          </div>
          {release.last_deployed_at && (
            <div className="flex gap-2">
              <span className="text-slate-500 w-28">Last deployed</span>
              <span className="text-slate-300">{formatAge(release.last_deployed_at)} ago</span>
            </div>
          )}
        </div>
      </div>

      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Version</div>
        <div className="flex items-center gap-2 font-mono text-sm">
          <span className="text-slate-400">{release.chart_version}</span>
          {finding && (
            <>
              <span className="text-slate-600">→</span>
              <span className="text-green-400 font-semibold">{finding.latest_version}</span>
            </>
          )}
        </div>
        {release.app_version && (
          <div className="text-xs text-slate-500">App version: <span className="text-slate-400 font-mono">{release.app_version}</span></div>
        )}
      </div>

      {release.repo_url && (
        <a
          href={release.repo_url}
          target="_blank"
          rel="noopener noreferrer"
          className="btn btn-secondary w-full justify-center"
        >
          <ExternalLink size={14} />
          View chart repository
        </a>
      )}

      {error && (
        <div className="px-3 py-2 rounded bg-red-950/60 border border-red-900/50 text-red-400 text-xs">{error}</div>
      )}

      {canWrite && (
        <>
          <div className="panel p-3 space-y-2">
            <div className="flex items-center gap-1.5 text-xs font-medium text-slate-400 uppercase tracking-wider">
              <ArrowUpCircle size={13} />
              Upgrade
            </div>
            <div className="space-y-1">
              <label className="block text-xs text-slate-500">Chart version</label>
              <input
                className="input w-full font-mono text-xs"
                placeholder={release.chart_version}
                value={chartVersion}
                onChange={(e) => setChartVersion(e.target.value)}
              />
              {finding && !chartVersion && (
                <button
                  className="text-xs text-blue-400 hover:text-blue-300"
                  onClick={() => setChartVersion(finding.latest_version)}
                >
                  Use latest ({finding.latest_version})
                </button>
              )}
            </div>
            <div className="space-y-1">
              <label className="block text-xs text-slate-500">
                Values (JSON) <span className="text-slate-600">— leave blank to keep the current values</span>
              </label>
              <textarea
                className="input w-full font-mono text-xs h-40 resize-y"
                placeholder={valuesPlaceholder}
                value={valuesText}
                onChange={(e) => setValuesText(e.target.value)}
              />
              {valuesError && <div className="text-xs text-red-400">{valuesError}</div>}
            </div>
            <button className="btn btn-primary w-full justify-center" disabled={busy} onClick={handleUpgrade}>
              {busy ? 'Upgrading...' : 'Upgrade'}
            </button>
          </div>

          <div className="panel p-3 space-y-2">
            <div className="flex items-center gap-1.5 text-xs font-medium text-slate-400 uppercase tracking-wider">
              <History size={13} />
              Rollback
            </div>
            <div className="flex items-center gap-2">
              <input
                className="input flex-1 font-mono text-xs"
                type="number"
                min={1}
                placeholder={`e.g. ${Math.max(1, release.revision - 1)}`}
                value={rollbackRevision}
                onChange={(e) => setRollbackRevision(e.target.value)}
              />
              <button className="btn btn-secondary flex-shrink-0" disabled={busy} onClick={handleRollback}>
                {busy ? 'Rolling back...' : 'Rollback'}
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

export function HelmPage() {
  const { user } = useAuth()
  const canWrite = user?.role === 'admin' || user?.role === 'operator'
  const { data: clusters = [] } = useClusters()
  const [clusterId, setClusterId] = useState('')
  const [activeReleaseId, setActiveReleaseId] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['helm', clusterId],
    queryFn: () =>
      getHelmReleases({
        cluster_id: clusterId || undefined,
        limit: 100,
      }),
  })

  const releases = data?.data ?? []

  const columns: Column<HelmRelease>[] = [
    {
      key: 'name',
      header: 'Release',
      render: (r) => (
        <div>
          <span className="text-xs font-medium text-slate-200">{r.name}</span>
          <span className="ml-2 text-xs text-slate-500 font-mono">{r.chart_name}</span>
        </div>
      ),
    },
    {
      key: 'version',
      header: 'Chart Version',
      render: (r) => <span className="font-mono text-xs text-slate-400">{r.chart_version}</span>,
    },
    {
      key: 'app_version',
      header: 'App Version',
      render: (r) => (
        <span className="font-mono text-xs text-slate-400">{r.app_version || '—'}</span>
      ),
    },
    {
      key: 'location',
      header: 'Cluster / Namespace',
      render: (r) => (
        <div className="flex flex-col gap-0.5">
          <span className="text-xs text-slate-300 font-mono">{r.cluster?.name}</span>
          <span className="text-xs text-slate-500">{r.namespace_name}</span>
        </div>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      width: '90px',
      render: (r) => (
        <span className={clsx('text-xs font-medium capitalize', HELM_STATUS_STYLES[r.status] || 'text-slate-400')}>
          {r.status}
        </span>
      ),
    },
    {
      key: 'deployed',
      header: 'Deployed',
      width: '80px',
      render: (r) => (
        <span className="text-xs text-slate-500">{r.last_deployed_at ? formatAge(r.last_deployed_at) : '—'}</span>
      ),
    },
  ]

  const activeRelease = releases.find((r) => r.id === activeReleaseId)

  return (
    <div className="flex flex-col h-full">
      {/* Filter bar */}
      <div className="flex items-center gap-3 px-4 py-2 border-b border-surface-border bg-surface-panel">
        <select
          className="input py-1 text-xs h-[28px]"
          value={clusterId}
          onChange={(e) => setClusterId(e.target.value)}
        >
          <option value="">All clusters</option>
          {clusters.map((c) => (
            <option key={c.id} value={c.id}>{c.display_name}</option>
          ))}
        </select>

        <span className="ml-auto text-xs text-slate-500">{releases.length} releases</span>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <DataTable
          columns={columns}
          data={releases}
          loading={isLoading}
          rowKey={(r) => r.id}
          onRowClick={(r) => setActiveReleaseId(r.id)}
          emptyMessage="No Helm releases found"
        />
      </div>

      {/* Detail slide-over */}
      <SlideOver
        open={!!activeReleaseId}
        onClose={() => setActiveReleaseId(null)}
        title={activeRelease?.name || ''}
        width="40%"
      >
        {activeReleaseId && <HelmDetail releaseId={activeReleaseId} canWrite={canWrite} />}
      </SlideOver>
    </div>
  )
}
