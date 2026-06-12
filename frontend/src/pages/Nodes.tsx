import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Filter } from 'lucide-react'
import clsx from 'clsx'
import { getNodes } from '../api/client'
import { useClusters } from '../hooks/useClusters'
import { DataTable, Column } from '../components/DataTable'
import { SlideOver } from '../components/SlideOver'
import type { Node, NodeCondition } from '../types'
import { formatAge } from '../utils/formatting'

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
