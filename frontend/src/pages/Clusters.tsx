import React, { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import * as Dialog from '@radix-ui/react-dialog'
import { Plus, Server, RefreshCw, Trash2, X, Terminal, FileKey } from 'lucide-react'
import clsx from 'clsx'
import { createCluster, syncCluster, deleteCluster, type CreateClusterPayload } from '../api/client'
import { useClusters } from '../hooks/useClusters'
import type { Cluster, ClusterStatus } from '../types'
import { formatRelative } from '../utils/formatting'

const STATUS_STYLES: Record<string, string> = {
  healthy: 'bg-green-900/50 text-green-300 border-green-900/70',
  degraded: 'bg-yellow-900/50 text-yellow-300 border-yellow-900/70',
  unreachable: 'bg-red-900/50 text-red-300 border-red-900/70',
  unknown: 'bg-slate-800 text-slate-400 border-slate-700',
}

function StatusPill({ status }: { status: ClusterStatus }) {
  return (
    <span className={clsx('px-2 py-0.5 text-xs rounded-full border capitalize', STATUS_STYLES[status] || STATUS_STYLES.unknown)}>
      {status}
    </span>
  )
}

function ModeBadge({ cluster }: { cluster: Cluster }) {
  const isSSH = cluster.connection_mode === 'ssh'
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-slate-400">
      {isSSH ? <Terminal size={12} className="text-blue-400" /> : <FileKey size={12} className="text-slate-500" />}
      {isSSH ? `ssh · ${cluster.ssh_user}@${cluster.ssh_host}` : cluster.connection_mode || 'kubeconfig'}
    </span>
  )
}

function AddClusterModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [mode, setMode] = useState<'kubeconfig' | 'ssh'>('kubeconfig')
  const [error, setError] = useState('')

  // kubeconfig mode
  const [kubeconfigRef, setKubeconfigRef] = useState('')
  const [apiEndpoint, setApiEndpoint] = useState('')
  const [tlsInsecure, setTlsInsecure] = useState(false)

  // ssh mode
  const [sshHost, setSshHost] = useState('')
  const [sshPort, setSshPort] = useState(22)
  const [sshUser, setSshUser] = useState('')
  const [sshPassword, setSshPassword] = useState('')
  const [sshKubeconfigPath, setSshKubeconfigPath] = useState('')
  const [sshSudo, setSshSudo] = useState(false)

  const { mutate, isPending } = useMutation({
    mutationFn: createCluster,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['clusters'] })
      onClose()
    },
    onError: (e: any) => setError(e?.response?.data?.error || e.message),
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')

    const payload: CreateClusterPayload = { name, connection_mode: mode }
    if (mode === 'kubeconfig') {
      payload.kubeconfig_ref = kubeconfigRef || undefined
      payload.api_endpoint = apiEndpoint || undefined
      payload.tls_insecure = tlsInsecure
    } else {
      payload.ssh_host = sshHost
      payload.ssh_port = sshPort
      payload.ssh_user = sshUser
      payload.ssh_password = sshPassword
      payload.ssh_kubeconfig_path = sshKubeconfigPath || undefined
      payload.ssh_sudo = sshSudo
    }
    mutate(payload)
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Cluster name</label>
        <input
          className="input w-full"
          placeholder="prod-eu-1"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
      </div>

      {/* Connection mode toggle */}
      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Connection mode</label>
        <div className="grid grid-cols-2 gap-2">
          <button
            type="button"
            onClick={() => setMode('kubeconfig')}
            className={clsx(
              'flex items-center gap-2 px-3 py-2 rounded border text-sm transition-colors',
              mode === 'kubeconfig'
                ? 'border-blue-500 bg-blue-950/40 text-slate-100'
                : 'border-surface-border text-slate-400 hover:bg-surface-elevated/60'
            )}
          >
            <FileKey size={14} /> Kubeconfig
          </button>
          <button
            type="button"
            onClick={() => setMode('ssh')}
            className={clsx(
              'flex items-center gap-2 px-3 py-2 rounded border text-sm transition-colors',
              mode === 'ssh'
                ? 'border-blue-500 bg-blue-950/40 text-slate-100'
                : 'border-surface-border text-slate-400 hover:bg-surface-elevated/60'
            )}
          >
            <Terminal size={14} /> SSH
          </button>
        </div>
      </div>

      {mode === 'kubeconfig' ? (
        <>
          <div className="space-y-1.5">
            <label className="block text-xs font-medium text-slate-400">
              Kubeconfig <span className="text-slate-600">(paste raw YAML or base64)</span>
            </label>
            <textarea
              className="input w-full font-mono text-xs h-28 resize-y"
              placeholder="apiVersion: v1&#10;clusters: ..."
              value={kubeconfigRef}
              onChange={(e) => setKubeconfigRef(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <label className="block text-xs font-medium text-slate-400">API endpoint <span className="text-slate-600">(optional)</span></label>
            <input
              className="input w-full"
              placeholder="https://10.0.0.1:6443"
              value={apiEndpoint}
              onChange={(e) => setApiEndpoint(e.target.value)}
            />
          </div>
          <label className="flex items-center gap-2 text-xs text-slate-400">
            <input type="checkbox" checked={tlsInsecure} onChange={(e) => setTlsInsecure(e.target.checked)} />
            Skip TLS verification (insecure)
          </label>
        </>
      ) : (
        <>
          <div className="rounded bg-blue-950/30 border border-blue-900/40 px-3 py-2 text-xs text-slate-400">
            KubePilot will SSH to the node, read its kubeconfig and tunnel Kubernetes API
            traffic through the SSH connection — useful when the API server is not reachable directly.
          </div>
          <div className="grid grid-cols-3 gap-2">
            <div className="col-span-2 space-y-1.5">
              <label className="block text-xs font-medium text-slate-400">SSH host</label>
              <input className="input w-full" placeholder="10.0.0.12" value={sshHost} onChange={(e) => setSshHost(e.target.value)} required />
            </div>
            <div className="space-y-1.5">
              <label className="block text-xs font-medium text-slate-400">Port</label>
              <input className="input w-full" type="number" value={sshPort} onChange={(e) => setSshPort(Number(e.target.value))} />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div className="space-y-1.5">
              <label className="block text-xs font-medium text-slate-400">User</label>
              <input className="input w-full" placeholder="ubuntu" value={sshUser} onChange={(e) => setSshUser(e.target.value)} required />
            </div>
            <div className="space-y-1.5">
              <label className="block text-xs font-medium text-slate-400">Password</label>
              <input className="input w-full" type="password" value={sshPassword} onChange={(e) => setSshPassword(e.target.value)} required />
            </div>
          </div>
          <div className="space-y-1.5">
            <label className="block text-xs font-medium text-slate-400">
              Remote kubeconfig path <span className="text-slate-600">(optional)</span>
            </label>
            <input
              className="input w-full font-mono text-xs"
              placeholder="/etc/rancher/k3s/k3s.yaml (auto-detected if empty)"
              value={sshKubeconfigPath}
              onChange={(e) => setSshKubeconfigPath(e.target.value)}
            />
          </div>
          <label className="flex items-center gap-2 text-xs text-slate-400">
            <input type="checkbox" checked={sshSudo} onChange={(e) => setSshSudo(e.target.checked)} />
            Read kubeconfig with sudo (needed for root-only files)
          </label>
        </>
      )}

      {error && (
        <div className="px-3 py-2 rounded bg-red-950/60 border border-red-900/50 text-red-400 text-xs">
          {error}
        </div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
        <button type="submit" className="btn btn-primary" disabled={isPending}>
          {isPending ? 'Adding...' : 'Add cluster'}
        </button>
      </div>
    </form>
  )
}

export function Clusters() {
  const qc = useQueryClient()
  const { data: clusters = [], isLoading } = useClusters()
  const [addOpen, setAddOpen] = useState(false)
  const [busyId, setBusyId] = useState<string | null>(null)

  async function handleSync(id: string) {
    setBusyId(id)
    try {
      await syncCluster(id)
      await qc.invalidateQueries({ queryKey: ['clusters'] })
    } finally {
      setBusyId(null)
    }
  }

  async function handleDelete(id: string) {
    if (!confirm('Delete this cluster? This removes it from KubePilot (the cluster itself is untouched).')) return
    await deleteCluster(id)
    await qc.invalidateQueries({ queryKey: ['clusters'] })
  }

  return (
    <div className="p-4 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-base font-semibold text-slate-100">Clusters</h1>
        <Dialog.Root open={addOpen} onOpenChange={setAddOpen}>
          <Dialog.Trigger asChild>
            <button className="btn btn-primary">
              <Plus size={14} />
              Add cluster
            </button>
          </Dialog.Trigger>
          <Dialog.Portal>
            <Dialog.Overlay className="fixed inset-0 bg-black/50 z-40" />
            <Dialog.Content className="fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-lg bg-surface-panel border border-surface-border rounded-xl shadow-2xl p-6 max-h-[90vh] overflow-y-auto">
              <div className="flex items-center justify-between mb-4">
                <Dialog.Title className="text-sm font-semibold text-slate-100">Add Cluster</Dialog.Title>
                <Dialog.Close asChild>
                  <button className="p-1 rounded hover:bg-surface-elevated text-slate-400 hover:text-slate-200">
                    <X size={15} />
                  </button>
                </Dialog.Close>
              </div>
              <AddClusterModal onClose={() => setAddOpen(false)} />
            </Dialog.Content>
          </Dialog.Portal>
        </Dialog.Root>
      </div>

      {isLoading ? (
        <div className="panel p-4 space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="skeleton h-10 w-full rounded" />)}
        </div>
      ) : clusters.length === 0 ? (
        <div className="panel p-12 flex flex-col items-center gap-4">
          <Server size={32} className="text-slate-600" />
          <div className="text-center">
            <p className="text-sm text-slate-400">No clusters registered</p>
            <p className="text-xs text-slate-600 mt-1">Connect a cluster via kubeconfig or SSH to start collecting</p>
          </div>
          <button className="btn btn-primary" onClick={() => setAddOpen(true)}>
            <Plus size={14} />
            Add first cluster
          </button>
        </div>
      ) : (
        <div className="panel overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-slate-500 border-b border-surface-border">
                <th className="px-4 py-2.5 font-medium">Name</th>
                <th className="px-4 py-2.5 font-medium">Environment</th>
                <th className="px-4 py-2.5 font-medium">Connection</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
                <th className="px-4 py-2.5 font-medium">Last seen</th>
                <th className="px-4 py-2.5 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {clusters.map((cluster) => (
                <tr key={cluster.id} className="border-b border-surface-border/60 last:border-0 hover:bg-surface-elevated/40">
                  <td className="px-4 py-2.5 font-medium text-slate-200">{cluster.name}</td>
                  <td className="px-4 py-2.5 text-slate-400">{cluster.environment?.name || '—'}</td>
                  <td className="px-4 py-2.5"><ModeBadge cluster={cluster} /></td>
                  <td className="px-4 py-2.5"><StatusPill status={cluster.status} /></td>
                  <td className="px-4 py-2.5 text-slate-500 text-xs">
                    {cluster.last_seen_at ? formatRelative(cluster.last_seen_at) : 'never'}
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center justify-end gap-1.5">
                      <button
                        className="btn btn-secondary py-1 px-2 text-xs"
                        onClick={() => handleSync(cluster.id)}
                        disabled={busyId === cluster.id}
                        title="Trigger a collection pass"
                      >
                        <RefreshCw size={12} className={busyId === cluster.id ? 'animate-spin' : ''} />
                      </button>
                      <button
                        className="btn btn-danger py-1 px-2 text-xs"
                        onClick={() => handleDelete(cluster.id)}
                        title="Remove cluster"
                      >
                        <Trash2 size={12} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
