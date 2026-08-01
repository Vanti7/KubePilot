import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, RefreshCw } from 'lucide-react'
import clsx from 'clsx'
import { getCluster, syncCluster, getFindings, getClusterResources, getNodes } from '../api/client'
import { SeverityBadge } from '../components/SeverityBadge'
import { ClusterStatusIcon } from '../components/ClusterStatusBadge'
import { ResourceGauge, bytesToHuman } from '../components/ResourceGauge'
import { DataTable, Column } from '../components/DataTable'
import type { UpdateFinding, Node, NodeCondition } from '../types'
import { formatAge, formatRelative, scoreToColor } from '../utils/formatting'

const STATUS_COLOR = {
  healthy: 'text-green-400',
  unreachable: 'text-red-400',
  degraded: 'text-yellow-400',
  unknown: 'text-slate-500',
}

function nodeReady(conditions: NodeCondition[]): boolean {
  return conditions.find((c) => c.type === 'Ready')?.status === 'True'
}

export function ClusterDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const { data: cluster, isLoading: clusterLoading } = useQuery({
    queryKey: ['cluster', id],
    queryFn: () => getCluster(id!),
    enabled: !!id,
  })

  const { data: resources } = useQuery({
    queryKey: ['cluster-resources', id],
    queryFn: () => getClusterResources(id!),
    enabled: !!id,
    refetchInterval: 30000,
  })

  const { data: nodesData } = useQuery({
    queryKey: ['nodes', { cluster_id: id }],
    queryFn: () => getNodes({ cluster_id: id!, limit: 100 }),
    enabled: !!id,
  })

  const nodes = nodesData?.data ?? []

  const { data: findingsData, isLoading: findingsLoading } = useQuery({
    queryKey: ['findings', { cluster_id: id }],
    queryFn: () => getFindings({ cluster_id: id!, limit: 50 }),
    enabled: !!id,
  })

  const findings = findingsData?.data ?? []

  const nodeColumns: Column<Node>[] = [
    {
      key: 'name',
      header: 'Node',
      render: (n) => <span className="text-xs font-mono font-medium text-slate-200">{n.name}</span>,
    },
    {
      key: 'role',
      header: 'Role',
      width: '120px',
      render: (n) => (
        <span className="text-xs text-slate-400">
          {n.role === 'control-plane' || n.role === 'master' ? 'control-plane' : 'worker'}
        </span>
      ),
    },
    {
      key: 'kubelet',
      header: 'Kubelet',
      width: '110px',
      render: (n) => <span className="font-mono text-xs text-slate-400">{n.kubelet_version}</span>,
    },
    {
      key: 'status',
      header: 'Status',
      width: '90px',
      render: (n) => (
        <span className={clsx('inline-flex items-center gap-1.5 text-xs', nodeReady(n.conditions) ? 'text-green-400' : 'text-red-400')}>
          <span className={clsx('w-2 h-2 rounded-full', nodeReady(n.conditions) ? 'bg-green-400' : 'bg-red-400')} />
          {nodeReady(n.conditions) ? 'Ready' : 'NotReady'}
        </span>
      ),
    },
  ]

  const columns: Column<UpdateFinding>[] = [
    {
      key: 'severity',
      header: 'Sev',
      width: '70px',
      render: (f) => f.score_severity || f.severity ? <SeverityBadge severity={f.score_severity || f.severity} /> : <span className="text-slate-600">—</span>,
    },
    {
      key: 'resource',
      header: 'Resource',
      render: (f) => (
        <div className="flex items-center gap-1.5">
          <span className="text-xs text-slate-200 font-medium">{f.workload_name || f.helm_release_name || f.title}</span>
          <span className="text-xs text-slate-500">{f.workload_kind}</span>
        </div>
      ),
    },
    {
      key: 'update',
      header: 'Update',
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
        <span className={clsx('font-mono text-xs', scoreToColor(f.score ?? 0))}>
          {f.score ?? '—'}
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
          <ClusterStatusIcon status={cluster.status} size={14} />
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

      {/* System resources */}
      {resources && resources.nodes > 0 && (
        <div className="panel">
          <div className="flex items-center justify-between px-4 py-2.5 border-b border-surface-border">
            <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">System Resources</h2>
            <span className="text-xs text-slate-500 font-mono">
              {resources.nodes_ready}/{resources.nodes} nodes ready · {resources.pods_running} pods
            </span>
          </div>
          <div className="grid grid-cols-3 gap-3 p-4">
            <ResourceGauge
              label="CPU"
              percent={resources.cpu_usage_percent}
              detail={`${resources.cpu_used_cores.toFixed(1)} / ${resources.cpu_capacity_cores.toFixed(0)} cores`}
            />
            <ResourceGauge
              label="Memory"
              percent={resources.memory_usage_percent}
              detail={`${bytesToHuman(resources.memory_used_bytes)} / ${bytesToHuman(resources.memory_capacity_bytes)}`}
            />
            <ResourceGauge
              label="Disk"
              percent={resources.disk_usage_percent}
              detail={`${bytesToHuman(resources.disk_used_bytes)} / ${bytesToHuman(resources.disk_capacity_bytes)}`}
            />
          </div>
        </div>
      )}

      {/* Nodes */}
      {nodes.length > 0 && (
        <div className="panel">
          <div className="flex items-center justify-between px-4 py-2.5 border-b border-surface-border">
            <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">
              Nodes
              <span className="ml-2 text-slate-500 normal-case font-normal">{nodes.length}</span>
            </h2>
            <button
              className="text-xs text-blue-400 hover:text-blue-300 transition-colors"
              onClick={() => navigate('/nodes')}
            >
              View all nodes →
            </button>
          </div>
          <DataTable columns={nodeColumns} data={nodes} rowKey={(n) => n.id} />
        </div>
      )}

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
