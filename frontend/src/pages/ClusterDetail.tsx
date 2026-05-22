import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, RefreshCw, Wifi, WifiOff, AlertTriangle } from 'lucide-react'
import clsx from 'clsx'
import { getCluster, syncCluster, getFindings } from '../api/client'
import { SeverityBadge } from '../components/SeverityBadge'
import { DataTable, Column } from '../components/DataTable'
import type { UpdateFinding } from '../types'
import { formatAge, formatRelative, scoreToColor } from '../utils/formatting'

const STATUS_ICON = {
  connected: <Wifi size={14} className="text-green-400" />,
  unreachable: <WifiOff size={14} className="text-red-400" />,
  degraded: <AlertTriangle size={14} className="text-yellow-400" />,
}

const STATUS_COLOR = {
  connected: 'text-green-400',
  unreachable: 'text-red-400',
  degraded: 'text-yellow-400',
}

export function ClusterDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const { data: cluster, isLoading: clusterLoading } = useQuery({
    queryKey: ['cluster', id],
    queryFn: () => getCluster(id!),
    enabled: !!id,
  })

  const { data: findingsData, isLoading: findingsLoading } = useQuery({
    queryKey: ['findings', { cluster_id: id }],
    queryFn: () => getFindings({ cluster_id: id!, limit: 50 }),
    enabled: !!id,
  })

  const findings = findingsData?.data ?? []

  const columns: Column<UpdateFinding>[] = [
    {
      key: 'severity',
      header: 'Sev',
      width: '70px',
      render: (f) => f.risk_score ? <SeverityBadge severity={f.risk_score.severity} /> : <span className="text-slate-600">—</span>,
    },
    {
      key: 'resource',
      header: 'Resource',
      render: (f) => (
        <div className="flex items-center gap-1.5">
          <span className="text-xs text-slate-200 font-medium">{f.workload_name || f.target_id}</span>
          <span className="text-xs text-slate-500">{f.target_kind}</span>
        </div>
      ),
    },
    {
      key: 'update',
      header: 'Update',
      render: (f) => (
        <span className="font-mono text-xs text-slate-400">
          {f.current_version} <span className="text-slate-600">→</span>{' '}
          <span className="text-green-400">{f.available_version}</span>
        </span>
      ),
    },
    {
      key: 'score',
      header: 'Score',
      width: '60px',
      align: 'right',
      render: (f) => (
        <span className={clsx('font-mono text-xs', scoreToColor(f.risk_score?.score ?? 0))}>
          {f.risk_score?.score ?? '—'}
        </span>
      ),
    },
    {
      key: 'namespace',
      header: 'Namespace',
      render: (f) => <span className="font-mono text-xs text-slate-400">{f.namespace_name || '—'}</span>,
    },
    {
      key: 'age',
      header: 'Age',
      width: '60px',
      render: (f) => <span className="text-xs text-slate-500">{formatAge(f.first_detected_at)}</span>,
    },
  ]

  if (clusterLoading) {
    return (
      <div className="p-4">
        <div className="skeleton h-6 w-48 rounded mb-4" />
        <div className="skeleton h-32 rounded" />
      </div>
    )
  }

  if (!cluster) {
    return (
      <div className="p-4 text-slate-500 text-sm">Cluster not found.</div>
    )
  }

  return (
    <div className="p-4 space-y-4 max-w-6xl mx-auto">
      {/* Back + header */}
      <div className="flex items-center gap-3">
        <button
          className="p-1.5 rounded hover:bg-surface-elevated text-slate-400 hover:text-slate-200 transition-colors"
          onClick={() => navigate(-1)}
        >
          <ArrowLeft size={16} />
        </button>
        <div className="flex items-center gap-2">
          {STATUS_ICON[cluster.status]}
          <h1 className="text-base font-semibold text-slate-100">{cluster.display_name}</h1>
          <span className={clsx('text-xs capitalize', STATUS_COLOR[cluster.status])}>{cluster.status}</span>
        </div>
        <div className="ml-auto flex items-center gap-2">
          <span className="text-xs text-slate-500">Last seen {formatRelative(cluster.last_seen_at)}</span>
          <button
            className="btn btn-secondary py-1 px-2"
            onClick={() => syncCluster(cluster.id)}
          >
            <RefreshCw size={13} />
            Sync
          </button>
        </div>
      </div>

      {/* Cluster info */}
      <div className="grid grid-cols-3 gap-3">
        <div className="panel p-4">
          <div className="text-xs text-slate-500 mb-1">Kubernetes Version</div>
          <div className="text-sm font-mono font-semibold text-slate-100">{cluster.version || '—'}</div>
        </div>
        <div className="panel p-4">
          <div className="text-xs text-slate-500 mb-1">Endpoint</div>
          <div className="text-xs font-mono text-slate-300 truncate">{cluster.endpoint || '—'}</div>
        </div>
        <div className="panel p-4">
          <div className="text-xs text-slate-500 mb-1">Environment</div>
          <div className="text-sm font-semibold text-slate-100">{cluster.environment?.name || '—'}</div>
        </div>
      </div>

      {/* Findings for this cluster */}
      <div className="panel">
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-surface-border">
          <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">
            Update Findings
            {findingsData && <span className="ml-2 text-slate-500 normal-case font-normal">{findingsData.total}</span>}
          </h2>
          <button
            className="text-xs text-blue-400 hover:text-blue-300 transition-colors"
            onClick={() => navigate(`/updates?cluster=${id}`)}
          >
            View in updates →
          </button>
        </div>
        <DataTable
          columns={columns}
          data={findings}
          loading={findingsLoading}
          rowKey={(f) => f.id}
          emptyMessage="No findings for this cluster"
        />
      </div>
    </div>
  )
}
