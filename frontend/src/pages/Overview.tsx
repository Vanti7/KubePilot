import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { RefreshCw, Wifi, WifiOff, AlertTriangle } from 'lucide-react'
import clsx from 'clsx'
import { getOverview } from '../api/client'
import { SeverityBadge } from '../components/SeverityBadge'
import { DataTable, Column } from '../components/DataTable'
import type { UpdateFinding } from '../types'
import { formatAge, formatRelative, scoreToColor } from '../utils/formatting'

function StatCard({
  label,
  value,
  accent,
  sublabel,
}: {
  label: string
  value: number | string
  accent?: string
  sublabel?: string
}) {
  return (
    <div className="stat-card">
      <div className={clsx('text-2xl font-bold font-mono', accent || 'text-slate-100')}>
        {value}
      </div>
      <div className="text-xs text-slate-400 font-medium">{label}</div>
      {sublabel && <div className="text-xs text-slate-600">{sublabel}</div>}
    </div>
  )
}

const clusterStatusIcon = {
  healthy: <Wifi size={13} className="text-green-400" />,
  unreachable: <WifiOff size={13} className="text-red-400" />,
  degraded: <AlertTriangle size={13} className="text-yellow-400" />,
}

const clusterStatusColor = {
  healthy: 'text-green-400',
  unreachable: 'text-red-400',
  degraded: 'text-yellow-400',
}

export function Overview() {
  const navigate = useNavigate()
  const { data, isLoading, dataUpdatedAt, refetch, isFetching } = useQuery({
    queryKey: ['overview'],
    queryFn: getOverview,
    refetchInterval: 60000,
  })

  const topCriticalColumns: Column<UpdateFinding>[] = [
    {
      key: 'severity',
      header: 'Sev',
      width: '70px',
      render: (f) =>
        f.risk_score ? <SeverityBadge severity={f.risk_score.severity} /> : <span className="text-slate-600">—</span>,
    },
    {
      key: 'resource',
      header: 'Resource',
      render: (f) => (
        <div>
          <span className="text-slate-200 font-medium text-xs">{f.workload_name || f.target_id}</span>
          <span className="ml-1.5 text-xs text-slate-500">{f.target_kind}</span>
        </div>
      ),
    },
    {
      key: 'version',
      header: 'Version',
      render: (f) => (
        <span className="font-mono text-xs text-slate-400">
          {f.current_version} <span className="text-slate-600">→</span>{' '}
          <span className="text-green-400">{f.latest_version}</span>
        </span>
      ),
    },
    {
      key: 'score',
      header: 'Score',
      width: '60px',
      align: 'right',
      render: (f) => (
        <span className={clsx('font-mono text-xs font-semibold', scoreToColor(f.risk_score?.score ?? 0))}>
          {f.risk_score?.score ?? '—'}
        </span>
      ),
    },
    {
      key: 'cluster',
      header: 'Cluster',
      render: (f) => <span className="text-xs text-slate-400 font-mono">{f.cluster_name || '—'}</span>,
    },
    {
      key: 'age',
      header: 'Age',
      width: '60px',
      render: (f) => <span className="text-xs text-slate-500">{formatAge(f.first_detected_at)}</span>,
    },
  ]

  type ClusterRow = {
    id: string
    name: string
    display_name: string
    environment?: { name: string; color: string }
    status: 'healthy' | 'unreachable' | 'degraded'
    version: string
    last_seen_at: string
    counts: Record<string, number>
  }

  const clusterRows: ClusterRow[] = (data?.findings_per_cluster || []).map((c) => ({
    id: c.cluster_id,
    name: c.cluster_name,
    display_name: c.cluster_name,
    status: 'healthy' as const,
    version: '',
    last_seen_at: '',
    counts: { critical: c.critical, high: 0, medium: 0 },
  }))

  const clusterColumns: Column<ClusterRow>[] = [
    {
      key: 'name',
      header: 'Cluster',
      render: (r) => (
        <div className="flex items-center gap-2">
          {clusterStatusIcon[r.status]}
          <span className="text-xs font-medium text-slate-200">{r.display_name}</span>
        </div>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      render: (r) => (
        <span className={clsx('text-xs font-medium capitalize', clusterStatusColor[r.status])}>
          {r.status}
        </span>
      ),
    },
    {
      key: 'critical',
      header: 'Critical',
      width: '70px',
      align: 'right',
      render: (r) => (
        <span className={clsx('text-xs font-mono', r.counts.critical > 0 ? 'text-red-400' : 'text-slate-600')}>
          {r.counts.critical || 0}
        </span>
      ),
    },
    {
      key: 'high',
      header: 'High',
      width: '60px',
      align: 'right',
      render: (r) => (
        <span className={clsx('text-xs font-mono', r.counts.high > 0 ? 'text-orange-400' : 'text-slate-600')}>
          {r.counts.high || 0}
        </span>
      ),
    },
    {
      key: 'medium',
      header: 'Med',
      width: '60px',
      align: 'right',
      render: (r) => (
        <span className={clsx('text-xs font-mono', r.counts.medium > 0 ? 'text-yellow-400' : 'text-slate-600')}>
          {r.counts.medium || 0}
        </span>
      ),
    },
  ]

  const lastUpdated = dataUpdatedAt ? formatRelative(new Date(dataUpdatedAt).toISOString()) : null

  return (
    <div className="p-4 space-y-4 max-w-7xl mx-auto">
      {/* Page header */}
      <div className="flex items-center justify-between">
        <h1 className="text-base font-semibold text-slate-100">Overview</h1>
        <div className="flex items-center gap-3">
          {lastUpdated && (
            <span className="text-xs text-slate-600">Updated {lastUpdated}</span>
          )}
          <button
            className="btn btn-secondary py-1 px-2"
            onClick={() => refetch()}
            disabled={isFetching}
          >
            <RefreshCw size={13} className={isFetching ? 'animate-spin' : ''} />
            Refresh
          </button>
        </div>
      </div>

      {/* Stat cards */}
      <div className="grid grid-cols-4 gap-3">
        <StatCard
          label="Total Clusters"
          value={data?.cluster_status.total ?? '—'}
          sublabel={`${data?.cluster_status.healthy ?? 0} healthy`}
        />
        <StatCard
          label="Critical Findings"
          value={data?.findings_summary.critical ?? '—'}
          accent="text-severity-critical"
        />
        <StatCard
          label="High Findings"
          value={data?.findings_summary.high ?? '—'}
          accent="text-severity-high"
        />
        <StatCard
          label="Total Open"
          value={data?.findings_summary.total ?? '—'}
          sublabel="open findings"
        />
      </div>

      {/* Top critical findings */}
      <div className="panel">
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-surface-border">
          <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">
            Top Critical Findings
          </h2>
          <button
            className="text-xs text-blue-400 hover:text-blue-300 transition-colors"
            onClick={() => navigate('/updates')}
          >
            View all →
          </button>
        </div>
        <DataTable
          columns={topCriticalColumns}
          data={data?.top_findings ?? []}
          loading={isLoading}
          rowKey={(f) => f.id}
          onRowClick={(f) => navigate(`/updates?finding=${f.id}`)}
        />
      </div>

      {/* Cluster status */}
      <div className="panel">
        <div className="px-4 py-2.5 border-b border-surface-border">
          <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">
            Cluster Status
          </h2>
        </div>
        <DataTable
          columns={clusterColumns}
          data={clusterRows}
          loading={isLoading}
          rowKey={(r) => r.id}
          onRowClick={(r) => navigate(`/clusters/${r.id}`)}
          emptyMessage="No cluster data available"
        />
      </div>
    </div>
  )
}
