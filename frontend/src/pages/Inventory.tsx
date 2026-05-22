import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, ChevronDown as ChevronDownIcon } from 'lucide-react'
import clsx from 'clsx'
import { getWorkloads, getNamespaces, getFindings } from '../api/client'
import { useClusters } from '../hooks/useClusters'
import { DataTable, Column } from '../components/DataTable'
import { SlideOver } from '../components/SlideOver'
import type { Workload, Namespace } from '../types'
import { formatAge } from '../utils/formatting'

function WorkloadKindBadge({ kind }: { kind: string }) {
  const colors: Record<string, string> = {
    Deployment: 'bg-blue-950/80 text-blue-400 border-blue-900/60',
    DaemonSet: 'bg-purple-950/80 text-purple-400 border-purple-900/60',
    StatefulSet: 'bg-orange-950/80 text-orange-400 border-orange-900/60',
    CronJob: 'bg-teal-950/80 text-teal-400 border-teal-900/60',
    Job: 'bg-slate-800 text-slate-400 border-slate-700',
  }
  return (
    <span className={clsx('px-1.5 py-0.5 rounded text-xs border font-medium', colors[kind] || 'bg-slate-800 text-slate-400 border-slate-700')}>
      {kind}
    </span>
  )
}

function WorkloadDetail({ workload }: { workload: Workload }) {
  const { data: findings } = useQuery({
    queryKey: ['workload-findings', workload.id],
    queryFn: () => getFindings({ cluster_id: workload.cluster_id }),
  })

  const relevantFindings = findings?.data.filter((f) => f.target_id === workload.id) ?? []

  return (
    <div className="p-4 space-y-4">
      <div className="panel p-3 space-y-2">
        <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Workload</div>
        <div className="space-y-1.5 text-xs">
          <div className="flex gap-2">
            <span className="text-slate-500 w-24">Name</span>
            <span className="text-slate-200 font-mono">{workload.name}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-24">Kind</span>
            <WorkloadKindBadge kind={workload.kind} />
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-24">Namespace</span>
            <span className="text-slate-300 font-mono">{workload.namespace_name}</span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-24">Replicas</span>
            <span className={clsx('font-mono', workload.replicas_ready < workload.replicas_desired ? 'text-yellow-400' : 'text-green-400')}>
              {workload.replicas_ready}/{workload.replicas_desired}
            </span>
          </div>
          <div className="flex gap-2">
            <span className="text-slate-500 w-24">Last seen</span>
            <span className="text-slate-300">{formatAge(workload.last_observed_at)} ago</span>
          </div>
        </div>
      </div>

      {Object.keys(workload.labels).length > 0 && (
        <div className="panel p-3 space-y-2">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">Labels</div>
          <div className="flex flex-wrap gap-1">
            {Object.entries(workload.labels).slice(0, 12).map(([k, v]) => (
              <span key={k} className="px-1.5 py-0.5 rounded bg-surface-elevated border border-surface-border text-xs font-mono text-slate-400">
                {k}={v}
              </span>
            ))}
          </div>
        </div>
      )}

      {relevantFindings.length > 0 && (
        <div className="panel p-3 space-y-2">
          <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
            Open Findings ({relevantFindings.length})
          </div>
          <div className="space-y-1.5">
            {relevantFindings.map((f) => (
              <div key={f.id} className="flex items-center gap-2 text-xs p-2 rounded bg-surface-elevated">
                <span className="font-mono text-slate-400">{f.current_version} → {f.available_version}</span>
                <span className={clsx('ml-auto font-mono font-semibold', f.risk_score?.score ? 'text-red-400' : '')}>
                  {f.risk_score?.score ?? ''}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

interface TreeNode {
  clusterId: string
  clusterName: string
  namespaces: Namespace[]
}

export function Inventory() {
  const { data: clusters = [] } = useClusters()
  const [expandedClusters, setExpandedClusters] = useState<Set<string>>(new Set())
  const [selectedNamespaceId, setSelectedNamespaceId] = useState<string | null>(null)
  const [selectedClusterId, setSelectedClusterId] = useState<string | null>(null)
  const [activeWorkload, setActiveWorkload] = useState<Workload | null>(null)

  const { data: namespaces = [] } = useQuery({
    queryKey: ['namespaces'],
    queryFn: () => getNamespaces(),
  })

  const { data: workloadsData, isLoading } = useQuery({
    queryKey: ['workloads', selectedNamespaceId, selectedClusterId],
    queryFn: () =>
      getWorkloads({
        namespace_id: selectedNamespaceId || undefined,
        cluster_id: selectedClusterId || undefined,
        limit: 100,
      }),
  })

  const workloads = workloadsData?.data ?? []

  function toggleCluster(id: string) {
    setExpandedClusters((prev) => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  const treeNodes: TreeNode[] = clusters.map((c) => ({
    clusterId: c.id,
    clusterName: c.display_name,
    namespaces: namespaces.filter((n) => n.cluster_id === c.id),
  }))

  const columns: Column<Workload>[] = [
    {
      key: 'name',
      header: 'Name',
      render: (w) => (
        <div className="flex items-center gap-2">
          <span className="text-xs font-medium text-slate-200">{w.name}</span>
          <WorkloadKindBadge kind={w.kind} />
        </div>
      ),
    },
    {
      key: 'replicas',
      header: 'Replicas',
      width: '90px',
      align: 'center',
      render: (w) => (
        <span className={clsx('font-mono text-xs', w.replicas_ready < w.replicas_desired ? 'text-yellow-400' : 'text-green-400')}>
          {w.replicas_ready}/{w.replicas_desired}
        </span>
      ),
    },
    {
      key: 'namespace',
      header: 'Namespace',
      render: (w) => <span className="font-mono text-xs text-slate-400">{w.namespace_name}</span>,
    },
    {
      key: 'findings',
      header: 'Updates',
      width: '70px',
      align: 'center',
      render: (w) =>
        w.open_findings_count ? (
          <span className="px-1.5 py-0.5 rounded-full bg-orange-950/60 text-orange-400 border border-orange-900/50 text-xs font-semibold">
            {w.open_findings_count}
          </span>
        ) : (
          <span className="text-slate-600 text-xs">—</span>
        ),
    },
    {
      key: 'last_seen',
      header: 'Last Seen',
      width: '80px',
      render: (w) => <span className="text-xs text-slate-500">{formatAge(w.last_observed_at)}</span>,
    },
  ]

  return (
    <div className="flex h-full">
      {/* Namespace tree */}
      <div className="w-56 flex-shrink-0 border-r border-surface-border overflow-y-auto bg-surface-panel">
        <div className="px-3 py-2 border-b border-surface-border">
          <span className="text-xs font-medium text-slate-400 uppercase tracking-wider">Namespaces</span>
        </div>

        {/* All */}
        <button
          className={clsx(
            'w-full text-left px-3 py-1.5 text-xs transition-colors',
            !selectedNamespaceId && !selectedClusterId
              ? 'bg-surface-elevated text-slate-200'
              : 'text-slate-400 hover:text-slate-200 hover:bg-surface-elevated/50'
          )}
          onClick={() => { setSelectedNamespaceId(null); setSelectedClusterId(null) }}
        >
          All workloads
        </button>

        {treeNodes.map((node) => (
          <div key={node.clusterId}>
            <button
              className="w-full flex items-center gap-1.5 px-3 py-1.5 text-xs text-slate-300 hover:bg-surface-elevated/50 transition-colors"
              onClick={() => {
                toggleCluster(node.clusterId)
                setSelectedClusterId(node.clusterId)
                setSelectedNamespaceId(null)
              }}
            >
              {expandedClusters.has(node.clusterId)
                ? <ChevronDownIcon size={12} className="flex-shrink-0 text-slate-500" />
                : <ChevronRight size={12} className="flex-shrink-0 text-slate-500" />
              }
              <span className="truncate font-medium">{node.clusterName}</span>
              <span className="ml-auto text-slate-600">{node.namespaces.length}</span>
            </button>

            {expandedClusters.has(node.clusterId) &&
              node.namespaces.map((ns) => (
                <button
                  key={ns.id}
                  className={clsx(
                    'w-full text-left pl-7 pr-3 py-1.5 text-xs transition-colors truncate',
                    selectedNamespaceId === ns.id
                      ? 'bg-surface-elevated text-slate-200'
                      : 'text-slate-500 hover:text-slate-300 hover:bg-surface-elevated/40'
                  )}
                  onClick={() => {
                    setSelectedNamespaceId(ns.id)
                    setSelectedClusterId(null)
                  }}
                >
                  {ns.name}
                </button>
              ))}
          </div>
        ))}
      </div>

      {/* Workloads table */}
      <div className="flex-1 overflow-auto">
        <div className="flex items-center justify-between px-4 py-2 border-b border-surface-border bg-surface-panel">
          <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">
            Workloads
            {workloads.length > 0 && <span className="ml-2 text-slate-500 normal-case font-normal">{workloads.length}</span>}
          </h2>
        </div>

        <DataTable
          columns={columns}
          data={workloads}
          loading={isLoading}
          rowKey={(w) => w.id}
          onRowClick={(w) => setActiveWorkload(w)}
          emptyMessage="No workloads found"
        />
      </div>

      {/* Workload detail slide-over */}
      <SlideOver
        open={!!activeWorkload}
        onClose={() => setActiveWorkload(null)}
        title={activeWorkload?.name || ''}
        width="38%"
      >
        {activeWorkload && <WorkloadDetail workload={activeWorkload} />}
      </SlideOver>
    </div>
  )
}
