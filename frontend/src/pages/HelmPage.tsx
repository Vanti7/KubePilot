import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ExternalLink, Copy, Filter } from 'lucide-react'
import clsx from 'clsx'
import { getHelmReleases } from '../api/client'
import { useClusters } from '../hooks/useClusters'
import { DataTable, Column } from '../components/DataTable'
import { SlideOver } from '../components/SlideOver'
import type { HelmRelease, UpdateType } from '../types'
import { formatAge, updateTypeLabel } from '../utils/formatting'

const UPDATE_TYPE_STYLES: Record<UpdateType, string> = {
  major: 'bg-red-950/60 text-red-400 border border-red-900/50',
  minor: 'bg-yellow-950/60 text-yellow-400 border border-yellow-900/50',
  patch: 'bg-green-950/60 text-green-400 border border-green-900/50',
  unknown: 'bg-slate-800 text-slate-400 border border-slate-700',
}

const HELM_STATUS_STYLES: Record<string, string> = {
  deployed: 'text-green-400',
  failed: 'text-red-400',
  pending: 'text-yellow-400',
  superseded: 'text-slate-500',
  uninstalled: 'text-slate-600',
}

function guessUpdateType(current: string, available: string): UpdateType {
  if (!current || !available) return 'unknown'
  const curr = current.replace(/^v/, '').split('.').map(Number)
  const avail = available.replace(/^v/, '').split('.').map(Number)
  if (avail[0] > curr[0]) return 'major'
  if (avail[1] > curr[1]) return 'minor'
  if (avail[2] > curr[2]) return 'patch'
  return 'unknown'
}

function HelmDetail({ release }: { release: HelmRelease }) {
  const updateType = release.available_version
    ? guessUpdateType(release.chart_version, release.available_version)
    : 'unknown'

  const upgradeCmd = `helm upgrade ${release.release_name} <repo>/${release.chart_name} --version ${release.available_version ?? release.chart_version} -n ${release.namespace_name}`

  function copy(text: string) {
    navigator.clipboard.writeText(text).catch(() => {})
  }

  return (
    <div className="p-4 space-y-4">
      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Release</div>
        <div className="space-y-1.5 text-xs">
          <div className="flex gap-2">
            <span className="text-slate-500 w-28">Release name</span>
            <span className="text-slate-200 font-mono">{release.release_name}</span>
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
            <span className="text-slate-300 font-mono">{release.cluster_name}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-28">Status</span>
            <span className={clsx('font-medium capitalize', HELM_STATUS_STYLES[release.status] || 'text-slate-400')}>
              {release.status}
            </span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-28">Last deployed</span>
            <span className="text-slate-300">{formatAge(release.last_deployed_at)} ago</span>
          </div>
        </div>
      </div>

      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Version</div>
        <div className="flex items-center gap-2 font-mono text-sm">
          <span className="text-slate-400">{release.chart_version}</span>
          {release.available_version && (
            <>
              <span className="text-slate-600">→</span>
              <span className="text-green-400 font-semibold">{release.available_version}</span>
              <span className={clsx('px-1.5 py-0.5 rounded text-xs font-medium', UPDATE_TYPE_STYLES[updateType])}>
                {updateTypeLabel(updateType)}
              </span>
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

      {release.available_version && (
        <div className="panel p-3 space-y-2">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Upgrade Command</div>
          <div className="flex items-start gap-2">
            <code className="flex-1 text-xs font-mono text-green-400 bg-surface-elevated p-2 rounded break-all">
              {upgradeCmd}
            </code>
            <button
              className="btn btn-secondary flex-shrink-0 p-1.5"
              onClick={() => copy(upgradeCmd)}
              title="Copy"
            >
              <Copy size={14} />
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

export function HelmPage() {
  const { data: clusters = [] } = useClusters()
  const [clusterId, setClusterId] = useState('')
  const [hasUpdatesOnly, setHasUpdatesOnly] = useState(false)
  const [activeRelease, setActiveRelease] = useState<HelmRelease | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['helm', clusterId, hasUpdatesOnly],
    queryFn: () =>
      getHelmReleases({
        cluster_id: clusterId || undefined,
        has_updates: hasUpdatesOnly || undefined,
        limit: 100,
      }),
  })

  const releases = data?.data ?? []

  const columns: Column<HelmRelease>[] = [
    {
      key: 'release_name',
      header: 'Release',
      render: (r) => (
        <div>
          <span className="text-xs font-medium text-slate-200">{r.release_name}</span>
          <span className="ml-2 text-xs text-slate-500 font-mono">{r.chart_name}</span>
        </div>
      ),
    },
    {
      key: 'version',
      header: 'Chart Version',
      render: (r) => {
        const updateType = r.available_version
          ? guessUpdateType(r.chart_version, r.available_version)
          : null
        return (
          <div className="flex items-center gap-1.5">
            <span className="font-mono text-xs text-slate-400">{r.chart_version}</span>
            {r.available_version && (
              <>
                <span className="text-slate-600">→</span>
                <span className="font-mono text-xs text-green-400">{r.available_version}</span>
                {updateType && (
                  <span className={clsx('px-1 py-0.5 text-xs rounded font-medium', UPDATE_TYPE_STYLES[updateType])}>
                    {updateTypeLabel(updateType)}
                  </span>
                )}
              </>
            )}
          </div>
        )
      },
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
          <span className="text-xs text-slate-300 font-mono">{r.cluster_name}</span>
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
        <span className="text-xs text-slate-500">{formatAge(r.last_deployed_at)}</span>
      ),
    },
  ]

  return (
    <div className="flex flex-col h-full">
      {/* Filter bar */}
      <div className="flex items-center gap-3 px-4 py-2 border-b border-surface-border bg-surface-panel">
        <Filter size={13} className="text-slate-500" />
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

        <label className="flex items-center gap-1.5 cursor-pointer select-none">
          <input
            type="checkbox"
            className="rounded accent-blue-500"
            checked={hasUpdatesOnly}
            onChange={(e) => setHasUpdatesOnly(e.target.checked)}
          />
          <span className="text-xs text-slate-400">Updates available only</span>
        </label>

        <span className="ml-auto text-xs text-slate-500">{releases.length} releases</span>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <DataTable
          columns={columns}
          data={releases}
          loading={isLoading}
          rowKey={(r) => r.id}
          onRowClick={(r) => setActiveRelease(r)}
          emptyMessage="No Helm releases found"
        />
      </div>

      {/* Detail slide-over */}
      <SlideOver
        open={!!activeRelease}
        onClose={() => setActiveRelease(null)}
        title={activeRelease?.release_name || ''}
        width="40%"
      >
        {activeRelease && <HelmDetail release={activeRelease} />}
      </SlideOver>
    </div>
  )
}
