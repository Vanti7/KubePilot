import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Filter } from 'lucide-react'
import clsx from 'clsx'
import { getNodes, getNodeMetrics } from '../api/client'
import { useClusters } from '../hooks/useClusters'
import { DataTable, Column } from '../components/DataTable'
import { SlideOver } from '../components/SlideOver'
import type { Node, NodeCondition, NodeMetric } from '../types'
import { formatAge } from '../utils/formatting'

function formatRate(bytesPerSec: number): string {
  if (!bytesPerSec || bytesPerSec < 1) return '0 B/s'
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s']
  let v = bytesPerSec
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

// UsageBar renders a compact horizontal gauge for a percentage value, colored by
// threshold. Renders a dash when the value is unavailable (no metrics yet).
function UsageBar({ percent, label }: { percent?: number; label: string }) {
  if (percent == null) return <span className="text-xs text-slate-600">—</span>
  const color = percent >= 85 ? 'bg-red-400' : percent >= 60 ? 'bg-yellow-400' : 'bg-green-400'
  return (
    <div className="flex items-center gap-2" title={`${label}: ${percent.toFixed(0)}%`}>
      <div className="flex-1 h-1.5 rounded-full bg-slate-800 overflow-hidden min-w-[36px]">
        <div className={clsx('h-full rounded-full', color)} style={{ width: `${Math.min(percent, 100)}%` }} />
      </div>
      <span className="text-xs font-mono text-slate-400 w-8 text-right">{percent.toFixed(0)}%</span>
    </div>
  )
}

// Sparkline draws a normalized polyline over the given series, no charting lib.
function Sparkline({ values, color }: { values: number[]; color: string }) {
  if (values.length < 2) return <div className="h-10 flex items-center text-xs text-slate-600">Not enough data</div>
  const w = 240
  const h = 40
  const pad = 2
  const max = Math.max(...values)
  const min = Math.min(...values)
  const range = max - min || 1
  const pts = values
    .map((v, i) => {
      const x = pad + (i / (values.length - 1)) * (w - 2 * pad)
      const y = h - pad - ((v - min) / range) * (h - 2 * pad)
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    .join(' ')
  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="w-full h-10" preserveAspectRatio="none">
      <polyline points={pts} fill="none" stroke={color} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}

function MetricRow({ label, value, values, color }: { label: string; value: string; values: number[]; color: string }) {
  return (
    <div>
      <div className="flex justify-between text-xs mb-0.5">
        <span className="text-slate-500">{label}</span>
        <span className="font-mono text-slate-300">{value}</span>
      </div>
      <Sparkline values={values} color={color} />
    </div>
  )
}

// NodeMetricsPanel fetches and renders the node's recent usage time-series.
function NodeMetricsPanel({ nodeId }: { nodeId: string }) {
  const { data: metrics = [] } = useQuery({
    queryKey: ['node-metrics', nodeId],
    queryFn: () => getNodeMetrics(nodeId, '6h'),
    refetchInterval: 30000,
  })

  if (metrics.length === 0) {
    return (
      <div className="panel p-3">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider mb-2">Metrics</div>
        <div className="text-xs text-slate-600">No metrics collected yet.</div>
      </div>
    )
  }

  const last = metrics[metrics.length - 1]
  const series = (key: keyof NodeMetric) => metrics.map((m) => Number(m[key]) || 0)

  return (
    <div className="panel p-3 space-y-3">
      <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Metrics (6h)</div>
      <MetricRow label="CPU" value={`${last.cpu_usage_percent.toFixed(0)}%`} values={series('cpu_usage_percent')} color="#34d399" />
      <MetricRow label="Memory" value={`${last.memory_usage_percent.toFixed(0)}%`} values={series('memory_usage_percent')} color="#60a5fa" />
      <MetricRow label="Disk" value={`${last.fs_used_percent.toFixed(0)}%`} values={series('fs_used_percent')} color="#fbbf24" />
      <MetricRow label="Net In" value={formatRate(last.network_rx_rate)} values={series('network_rx_rate')} color="#a78bfa" />
      <MetricRow label="Net Out" value={formatRate(last.network_tx_rate)} values={series('network_tx_rate')} color="#f472b6" />
      <div className="flex justify-between text-xs pt-1">
        <span className="text-slate-500">Pods running</span>
        <span className="font-mono text-slate-300">{last.pods_running}</span>
      </div>
    </div>
  )
}

function NodeRoleBadge({ role }: { role: string }) {
  const isControl = role === 'control-plane' || role === 'master'
  return (
    <span
      className={clsx(
        'px-1.5 py-0.5 rounded text-xs border font-medium',
        isControl
          ? 'bg-purple-950/80 text-purple-400 border-purple-900/60'
          : 'bg-slate-800 text-slate-400 border-slate-700'
      )}
    >
      {isControl ? 'control-plane' : 'worker'}
    </span>
  )
}

function NodeStatusDot({ conditions }: { conditions: NodeCondition[] }) {
  const ready = conditions.find((c) => c.type === 'Ready')
  const isReady = ready?.status === 'True'
  return (
    <div className="flex items-center gap-1.5">
      <span className={clsx('w-2 h-2 rounded-full flex-shrink-0', isReady ? 'bg-green-400' : 'bg-red-400')} />
      <span className={clsx('text-xs', isReady ? 'text-green-400' : 'text-red-400')}>
        {isReady ? 'Ready' : 'NotReady'}
      </span>
    </div>
  )
}

function NodeDetail({ node }: { node: Node }) {
  return (
    <div className="p-4 space-y-4">
      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Node Info</div>
        <div className="space-y-1.5 text-xs">
          <div className="flex gap-2">
            <span className="text-slate-500 w-32">Name</span>
            <span className="text-slate-200 font-mono break-all">{node.name}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-32">Role</span>
            <NodeRoleBadge role={node.role} />
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-32">OS</span>
            <span className="text-slate-300">{node.os_image}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-32">Kernel</span>
            <span className="text-slate-300 font-mono">{node.kernel_version}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-32">Kubelet</span>
            <span className="text-slate-300 font-mono">{node.kubelet_version}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-32">Runtime</span>
            <span className="text-slate-300 font-mono">{node.container_runtime}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-32">Last seen</span>
            <span className="text-slate-300">{formatAge(node.last_seen_at)} ago</span>
          </div>
        </div>
      </div>

      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Conditions</div>
        <div className="space-y-1.5">
          {node.conditions.map((c, i) => (
            <div key={i} className="flex items-center gap-2 text-xs">
              <span className={clsx(
                'w-2 h-2 rounded-full flex-shrink-0',
                c.status === 'True' ? (c.type === 'Ready' ? 'bg-green-400' : 'bg-yellow-400') : 'bg-slate-600'
              )} />
              <span className="text-slate-400 w-24">{c.type}</span>
              <span className={clsx(c.status === 'True' ? 'text-slate-300' : 'text-slate-500')}>
                {c.status}
              </span>
              {c.reason && <span className="text-slate-600">— {c.reason}</span>}
            </div>
          ))}
        </div>
      </div>

      <NodeMetricsPanel nodeId={node.id} />

      <div className="grid grid-cols-2 gap-3">
        <div className="panel p-3 space-y-2">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Capacity</div>
          {Object.entries(node.capacity ?? {}).map(([k, v]) => (
            <div key={k} className="flex justify-between text-xs">
              <span className="text-slate-500">{k}</span>
              <span className="font-mono text-slate-300">{v}</span>
            </div>
          ))}
        </div>
        <div className="panel p-3 space-y-2">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Allocatable</div>
          {Object.entries(node.allocatable ?? {}).map(([k, v]) => (
            <div key={k} className="flex justify-between text-xs">
              <span className="text-slate-500">{k}</span>
              <span className="font-mono text-slate-300">{v}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

export function Nodes() {
  const { data: clusters = [] } = useClusters()
  const [clusterId, setClusterId] = useState('')
  const [roleFilter, setRoleFilter] = useState('')
  const [activeNode, setActiveNode] = useState<Node | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['nodes', clusterId, roleFilter],
    queryFn: () =>
      getNodes({
        cluster_id: clusterId || undefined,
        role: roleFilter || undefined,
        limit: 200,
      }),
    refetchInterval: 30000,
  })

  const nodes = data?.data ?? []

  const columns: Column<Node>[] = [
    {
      key: 'name',
      header: 'Node',
      render: (n) => (
        <div>
          <span className="text-xs font-mono font-medium text-slate-200">{n.name}</span>
        </div>
      ),
    },
    {
      key: 'role',
      header: 'Role',
      width: '130px',
      render: (n) => <NodeRoleBadge role={n.role} />,
    },
    {
      key: 'os',
      header: 'OS',
      render: (n) => <span className="text-xs text-slate-400">{n.os_image}</span>,
    },
    {
      key: 'kubelet',
      header: 'Kubelet',
      width: '110px',
      render: (n) => <span className="font-mono text-xs text-slate-400">{n.kubelet_version}</span>,
    },
    {
      key: 'runtime',
      header: 'Runtime',
      render: (n) => <span className="font-mono text-xs text-slate-400">{n.container_runtime}</span>,
    },
    {
      key: 'cpu',
      header: 'CPU',
      width: '110px',
      render: (n) => <UsageBar percent={n.metrics?.cpu_usage_percent} label="CPU" />,
    },
    {
      key: 'mem',
      header: 'Memory',
      width: '110px',
      render: (n) => <UsageBar percent={n.metrics?.memory_usage_percent} label="Memory" />,
    },
    {
      key: 'disk',
      header: 'Disk',
      width: '110px',
      render: (n) => <UsageBar percent={n.metrics?.fs_used_percent} label="Disk" />,
    },
    {
      key: 'status',
      header: 'Status',
      width: '100px',
      render: (n) => <NodeStatusDot conditions={n.conditions} />,
    },
    {
      key: 'last_seen',
      header: 'Last Seen',
      width: '80px',
      render: (n) => <span className="text-xs text-slate-500">{formatAge(n.last_seen_at)}</span>,
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

        <select
          className="input py-1 text-xs h-[28px]"
          value={roleFilter}
          onChange={(e) => setRoleFilter(e.target.value)}
        >
          <option value="">All roles</option>
          <option value="control-plane">Control plane</option>
          <option value="worker">Worker</option>
        </select>

        <span className="ml-auto text-xs text-slate-500">{nodes.length} nodes</span>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <DataTable
          columns={columns}
          data={nodes}
          loading={isLoading}
          rowKey={(n) => n.id}
          onRowClick={(n) => setActiveNode(n)}
          emptyMessage="No nodes found"
        />
      </div>

      {/* Node detail slide-over */}
      <SlideOver
        open={!!activeNode}
        onClose={() => setActiveNode(null)}
        title={activeNode?.name || ''}
        width="38%"
      >
        {activeNode && <NodeDetail node={activeNode} />}
      </SlideOver>
    </div>
  )
}
