import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronRight, ChevronDown as ChevronDownIcon } from 'lucide-react'
import clsx from 'clsx'
import { getSecrets, getNamespaces } from '../api/client'
import { useClusters } from '../hooks/useClusters'
import { DataTable, Column } from '../components/DataTable'
import type { Secret, Namespace } from '../types'
import { formatAge } from '../utils/formatting'
import { differenceInDays } from 'date-fns'

const SECRET_TYPE_LABELS: Record<string, string> = {
  'Opaque': 'Opaque',
  'kubernetes.io/tls': 'TLS',
  'kubernetes.io/dockerconfigjson': 'Docker',
  'kubernetes.io/service-account-token': 'SA Token',
  'kubernetes.io/basic-auth': 'Basic Auth',
  'kubernetes.io/ssh-auth': 'SSH Auth',
  'bootstrap.kubernetes.io/token': 'Bootstrap Token',
}

function AgeCell({ date }: { date?: string }) {
  if (!date) return <span className="text-slate-600 text-xs">—</span>

  const days = differenceInDays(new Date(), new Date(date))
  const color =
    days <= 30  ? 'text-green-400' :
    days <= 90  ? 'text-yellow-400' :
    days <= 180 ? 'text-orange-400' :
                  'text-red-400'

  return (
    <span className={clsx('text-xs font-mono', color)} title={new Date(date).toLocaleString()}>
      {formatAge(date)}
    </span>
  )
}

function SecretTypeBadge({ type }: { type: string }) {
  const colors: Record<string, string> = {
    'Opaque': 'bg-slate-800 text-slate-400 border-slate-700',
    'kubernetes.io/tls': 'bg-blue-950/80 text-blue-400 border-blue-900/60',
    'kubernetes.io/dockerconfigjson': 'bg-purple-950/80 text-purple-400 border-purple-900/60',
    'kubernetes.io/service-account-token': 'bg-teal-950/80 text-teal-400 border-teal-900/60',
    'kubernetes.io/basic-auth': 'bg-orange-950/80 text-orange-400 border-orange-900/60',
  }
  return (
    <span className={clsx('px-1.5 py-0.5 rounded text-xs border font-medium', colors[type] || 'bg-slate-800 text-slate-400 border-slate-700')}>
      {SECRET_TYPE_LABELS[type] ?? type}
    </span>
  )
}

interface TreeNode {
  clusterId: string
  clusterName: string
  namespaces: Namespace[]
}

export function Secrets() {
  const { data: clusters = [] } = useClusters()
  const [expandedClusters, setExpandedClusters] = useState<Set<string>>(new Set())
  const [selectedNamespace, setSelectedNamespace] = useState<string | null>(null)
  const [selectedClusterId, setSelectedClusterId] = useState<string | null>(null)

  const { data: namespaces = [] } = useQuery({
    queryKey: ['namespaces'],
    queryFn: () => getNamespaces(),
  })

  const { data: secretsData, isLoading } = useQuery({
    queryKey: ['secrets', selectedNamespace, selectedClusterId],
    queryFn: () =>
      getSecrets({
        namespace: selectedNamespace || undefined,
        cluster_id: selectedClusterId || undefined,
        limit: 200,
      }),
  })

  const secrets = secretsData?.data ?? []

  function toggleCluster(id: string) {
    setExpandedClusters((prev) => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  const treeNodes: TreeNode[] = clusters.map((c) => ({
    clusterId: c.id,
    clusterName: c.display_name || c.name,
    namespaces: namespaces.filter((n) => n.cluster_id === c.id),
  }))

  const columns: Column<Secret>[] = [
    {
      key: 'name',
      header: 'Name',
      render: (s) => <span className="text-xs font-medium font-mono text-slate-200">{s.name}</span>,
    },
    {
      key: 'namespace',
      header: 'Namespace',
      render: (s) => <span className="font-mono text-xs text-slate-400">{s.namespace_name}</span>,
    },
    {
      key: 'type',
      header: 'Type',
      width: '140px',
      render: (s) => <SecretTypeBadge type={s.type} />,
    },
    {
      key: 'keys',
      header: 'Keys',
      render: (s) => (
        <div className="flex flex-wrap gap-1">
          {s.keys.slice(0, 5).map((k) => (
            <span key={k} className="px-1 py-0.5 rounded bg-surface-elevated border border-surface-border text-xs font-mono text-slate-500">
              {k}
            </span>
          ))}
          {s.keys.length > 5 && (
            <span className="text-xs text-slate-600">+{s.keys.length - 5}</span>
          )}
        </div>
      ),
    },
    {
      key: 'k8s_created_at',
      header: 'Created',
      width: '80px',
      render: (s) => <AgeCell date={s.k8s_created_at} />,
    },
    {
      key: 'k8s_updated_at',
      header: 'Modified',
      width: '80px',
      render: (s) => <AgeCell date={s.k8s_updated_at} />,
    },
  ]

  return (
    <div className="flex h-full">
      {/* Namespace tree */}
      <div className="w-56 flex-shrink-0 border-r border-surface-border overflow-y-auto bg-surface-panel">
        <div className="px-3 py-2 border-b border-surface-border">
          <span className="text-xs font-medium text-slate-400 uppercase tracking-wider">Namespaces</span>
        </div>

        <button
          className={clsx(
            'w-full text-left px-3 py-1.5 text-xs transition-colors',
            !selectedNamespace && !selectedClusterId
              ? 'bg-surface-elevated text-slate-200'
              : 'text-slate-400 hover:text-slate-200 hover:bg-surface-elevated/50'
          )}
          onClick={() => { setSelectedNamespace(null); setSelectedClusterId(null) }}
        >
          All secrets
        </button>

        {treeNodes.map((node) => (
          <div key={node.clusterId}>
            <button
              className="w-full flex items-center gap-1.5 px-3 py-1.5 text-xs text-slate-300 hover:bg-surface-elevated/50 transition-colors"
              onClick={() => {
                toggleCluster(node.clusterId)
                setSelectedClusterId(node.clusterId)
                setSelectedNamespace(null)
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
                    selectedNamespace === ns.name
                      ? 'bg-surface-elevated text-slate-200'
                      : 'text-slate-500 hover:text-slate-300 hover:bg-surface-elevated/40'
                  )}
                  onClick={() => {
                    setSelectedNamespace(ns.name)
                    setSelectedClusterId(null)
                  }}
                >
                  {ns.name}
                </button>
              ))}
          </div>
        ))}
      </div>

      {/* Secrets table */}
      <div className="flex-1 overflow-auto">
        <div className="flex items-center justify-between px-4 py-2 border-b border-surface-border bg-surface-panel">
          <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">
            Secrets
            {secrets.length > 0 && <span className="ml-2 text-slate-500 normal-case font-normal">{secrets.length}</span>}
          </h2>
        </div>

        <DataTable
          columns={columns}
          data={secrets}
          loading={isLoading}
          rowKey={(s) => s.id}
          emptyMessage="No secrets found"
        />
      </div>
    </div>
  )
}
