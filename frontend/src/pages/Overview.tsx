import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { RefreshCw } from 'lucide-react'
import clsx from 'clsx'
import { getOverview } from '../api/client'
import { SeverityBadge } from '../components/SeverityBadge'
import { ClusterStatusBadge } from '../components/ClusterStatusBadge'
import { ResourceGauge, bytesToHuman } from '../components/ResourceGauge'
import { DataTable, Column } from '../components/DataTable'
import type { UpdateFinding, ClusterResources, ClusterStatus } from '../types'
import { formatAge, formatRelative, scoreToColor } from '../utils/formatting'

function MiniBar({ percent }: { percent: number }) {
  const color = percent >= 85 ? 'bg-red-400' : percent >= 60 ? 'bg-yellow-400' : 'bg-green-400'
  return (
    <div className="flex items-center gap-2">
      <div className="flex-1 h-1.5 rounded-full bg-slate-800 overflow-hidden min-w-[50px]">
        <div className={clsx('h-full rounded-full', color)} style={{ width: `${Math.min(percent, 100)}%` }} />
      </div>
      <span className="text-xs font-mono text-slate-400 w-9 text-right">{percent.toFixed(0)}%</span>
    </div>
  )
}

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
        f.score_severity || f.severity ? <SeverityBadge severity={f.score_severity || f.severity} /> : <span className="text-slate-600">—</span>,
    },
    {
      key: 'resource',
      header: 'Resource',
      render: (f) => (
        <div>
          <span className="text-slate-200 font-medium text-xs">{f.workload_name || f.helm_release_name || f.title}</span>
          <span className="ml-1.5 text-xs text-slate-500">{f.workload_kind}</span>
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
        <span className={clsx('font-mono text-xs font-semibold', scoreToColor(f.score ?? 0))}>
          {f.score ?? '—'}
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
    status: ClusterStatus
    critical: number
    high: number
    medium: number
  }

  const clusterRows: ClusterRow[] = (data?.findings_per_cluster || []).map((c) => ({
    id: c.cluster_id,
    name: c.cluster_name,
    status: c.status,
    critical: c.critical,
    high: c.high,
    medium: c.medium,
  }))

  const clusterColumns: Column<ClusterRow>[] = [
    {
      key: 'name',
      header: 'Cluster',
      render: (r) => <span className="text-xs font-medium text-slate-200">{r.name}</span>,
    },
    {
      key: 'status',
      header: 'Status',
      render: (r) => <ClusterStatusBadge status={r.status} />,
    },
    {
      key: 'critical',
      header: 'Critical',
      width: '70px',
      align: 'right',
      render: (r) => (
        <span className={clsx('text-xs font-mono', r.critical > 0 ? 'text-red-400' : 'text-slate-600')}>
          {r.critical || 0}
        </span>
      ),
    },
    {
      key: 'high',
      header: 'High',
      width: '60px',
      align: 'right',
      render: (r) => (
        <span className={clsx('text-xs font-mono', r.high > 0 ? 'text-orange-400' : 'text-slate-600')}>
          {r.high || 0}
        </span>
      ),
    },
    {
      key: 'medium',
      header: 'Med',
      width: '60px',
      align: 'right',
      render: (r) => (
        <span className={clsx('text-xs font-mono', r.medium > 0 ? 'text-yellow-400' : 'text-slate-600')}>
          {r.medium || 0}
        </span>
      ),
    },
  ]

  const resourceColumns: Column<ClusterResources>[] = [
    {
      key: 'cluster',
      header: 'Cluster',
      render: (r) => <span className="text-xs font-medium text-slate-200">{r.cluster_name}</span>,
    },
    {
      key: 'nodes',
      header: 'Nodes',
      width: '80px',
      render: (r) => (
        <span className="text-xs font-mono text-slate-400">
          {r.nodes_ready}/{r.nodes}
        </span>
      ),
    },
    {
      key: 'cpu',
      header: 'CPU',
      width: '130px',
      render: (r) => <MiniBar percent={r.cpu_usage_percent} />,
    },
    {
      key: 'memory',
      header: 'Memory',
      width: '130px',
      render: (r) => <MiniBar percent={r.memory_usage_percent} />,
    },
    {
      key: 'disk',
      header: 'Disk',
      width: '130px',
      render: (r) => <MiniBar percent={r.disk_usage_percent} />,
    },
    {
      key: 'pods',
      header: 'Pods',
      width: '60px',
      align: 'right',
      render: (r) => <span className="text-xs font-mono text-slate-400">{r.pods_running}</span>,
    },
  ]

  const sys = data?.system_resources
  const perCluster = data?.resources_per_cluster ?? []

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

      {/* System resources (cluster-wide capacity vs live usage) */}
      {!isLoading && (
        <div className="panel">
          <div className="flex items-center justify-between px-4 py-2.5 border-b border-surface-border">
            <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">
              System Resources
            </h2>
            {sys && sys.nodes > 0 && (
              <span className="text-xs text-slate-500 font-mono">
                {sys.nodes_ready}/{sys.nodes} nodes ready · {sys.pods_running} pods
              </span>
            )}
          </div>
          {sys && sys.nodes > 0 ? (
            <>
              <div className="grid grid-cols-3 gap-3 p-4">
                <ResourceGauge
                  label="CPU"
                  percent={sys.cpu_usage_percent}
                  detail={`${sys.cpu_used_cores.toFixed(1)} / ${sys.cpu_capacity_cores.toFixed(0)} cores`}
                />
                <ResourceGauge
                  label="Memory"
                  percent={sys.memory_usage_percent}
                  detail={`${bytesToHuman(sys.memory_used_bytes)} / ${bytesToHuman(sys.memory_capacity_bytes)}`}
                />
                <ResourceGauge
                  label="Disk"
                  percent={sys.disk_usage_percent}
                  detail={`${bytesToHuman(sys.disk_used_bytes)} / ${bytesToHuman(sys.disk_capacity_bytes)}`}
                />
              </div>
              {perCluster.length > 1 && (
                <DataTable
                  columns={resourceColumns}
                  data={perCluster}
                  rowKey={(r) => r.cluster_id}
                  onRowClick={(r) => navigate(`/clusters/${r.cluster_id}`)}
                />
              )}
            </>
          ) : (
            <div className="px-4 py-8 text-center">
              <p className="text-xs text-slate-500">No node metrics collected yet.</p>
              <p className="text-xs text-slate-600 mt-1">
                Metrics appear once a collector runs with <span className="font-mono">NODE_METRICS_ENABLED</span>.
              </p>
            </div>
          )}
        </div>
      )}

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
