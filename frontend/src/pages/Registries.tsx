import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as Dialog from '@radix-ui/react-dialog'
import { Plus, Boxes, PlayCircle, Trash2, Pencil, X, CheckCircle, XCircle, ShieldAlert } from 'lucide-react'
import clsx from 'clsx'
import {
  getRegistries,
  createRegistry,
  updateRegistry,
  testRegistry,
  deleteRegistry,
  getHelmRepositories,
  createHelmRepository,
  updateHelmRepository,
  testHelmRepository,
  deleteHelmRepository,
  type RegistryPayload,
  type HelmRepositoryPayload,
} from '../api/client'
import type {
  ImageRegistry,
  RegistryType,
  RegistryTestResult,
  HelmRepository,
  HelmRepositoryTestResult,
} from '../types'

const REGISTRY_TYPES: { value: RegistryType; label: string; hostHint: string }[] = [
  { value: 'generic', label: 'Generic (OCI)', hostHint: 'registry.example.com' },
  { value: 'harbor', label: 'Harbor', hostHint: 'harbor.example.com' },
  { value: 'dockerhub', label: 'Docker Hub', hostHint: 'docker.io' },
  { value: 'ghcr', label: 'GitHub (GHCR)', hostHint: 'ghcr.io' },
  { value: 'quay', label: 'Quay', hostHint: 'quay.io' },
  { value: 'ecr', label: 'AWS ECR', hostHint: '<account>.dkr.ecr.<region>.amazonaws.com' },
  { value: 'gcr', label: 'Google GCR/AR', hostHint: 'gcr.io' },
  { value: 'acr', label: 'Azure ACR', hostHint: '<name>.azurecr.io' },
]

function RegistryForm({ existing, onClose }: { existing?: ImageRegistry; onClose: () => void }) {
  const qc = useQueryClient()
  const isEdit = !!existing
  const [name, setName] = useState(existing?.name ?? '')
  const [host, setHost] = useState(existing?.host ?? '')
  const [type, setType] = useState<RegistryType>(existing?.type ?? 'generic')
  const [username, setUsername] = useState(existing?.username ?? '')
  const [password, setPassword] = useState('')
  const [tlsInsecure, setTlsInsecure] = useState(existing?.tls_insecure ?? false)
  const [error, setError] = useState('')

  const { mutate, isPending } = useMutation({
    mutationFn: (payload: RegistryPayload) =>
      isEdit ? updateRegistry(existing!.id, payload) : createRegistry(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['registries'] })
      onClose()
    },
    onError: (e: any) => setError(e?.response?.data?.error || e.message),
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    mutate({
      name,
      host,
      type,
      username: username || undefined,
      password: password || undefined,
      tls_insecure: tlsInsecure,
    })
  }

  const hostHint = REGISTRY_TYPES.find((t) => t.value === type)?.hostHint

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Display name</label>
        <input className="input w-full" placeholder="Harbor prod" value={name} onChange={(e) => setName(e.target.value)} required />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">Type</label>
          <select className="input w-full" value={type} onChange={(e) => setType(e.target.value as RegistryType)}>
            {REGISTRY_TYPES.map((t) => (
              <option key={t.value} value={t.value}>{t.label}</option>
            ))}
          </select>
        </div>
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">Host</label>
          <input className="input w-full font-mono text-xs" placeholder={hostHint} value={host} onChange={(e) => setHost(e.target.value)} required />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">Username <span className="text-slate-600">(optional)</span></label>
          <input className="input w-full" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" />
        </div>
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">
            Password / token {isEdit && <span className="text-slate-600">(leave blank to keep)</span>}
          </label>
          <input className="input w-full" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
        </div>
      </div>

      <label className="flex items-center gap-2 text-xs text-slate-400">
        <input type="checkbox" checked={tlsInsecure} onChange={(e) => setTlsInsecure(e.target.checked)} />
        Skip TLS verification (self-signed registries, e.g. internal Harbor)
      </label>

      {error && (
        <div className="px-3 py-2 rounded bg-red-950/60 border border-red-900/50 text-red-400 text-xs">{error}</div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
        <button type="submit" className="btn btn-primary" disabled={isPending}>
          {isPending ? 'Saving...' : isEdit ? 'Save' : 'Add registry'}
        </button>
      </div>
    </form>
  )
}

function RegistryModal({ open, existing, onOpenChange }: { open: boolean; existing?: ImageRegistry; onOpenChange: (o: boolean) => void }) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50 z-40" />
        <Dialog.Content className="fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-lg bg-surface-panel border border-surface-border rounded-xl shadow-2xl p-6 max-h-[90vh] overflow-y-auto">
          <div className="flex items-center justify-between mb-4">
            <Dialog.Title className="text-sm font-semibold text-slate-100">
              {existing ? 'Edit registry' : 'Add registry'}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button className="p-1 rounded hover:bg-surface-elevated text-slate-400 hover:text-slate-200">
                <X size={15} />
              </button>
            </Dialog.Close>
          </div>
          {open && <RegistryForm existing={existing} onClose={() => onOpenChange(false)} />}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function HelmRepoForm({ existing, onClose }: { existing?: HelmRepository; onClose: () => void }) {
  const qc = useQueryClient()
  const isEdit = !!existing
  const [name, setName] = useState(existing?.name ?? '')
  const [url, setUrl] = useState(existing?.url ?? '')
  const [username, setUsername] = useState(existing?.username ?? '')
  const [password, setPassword] = useState('')
  const [tlsInsecure, setTlsInsecure] = useState(existing?.tls_insecure ?? false)
  const [error, setError] = useState('')

  const { mutate, isPending } = useMutation({
    mutationFn: (payload: HelmRepositoryPayload) =>
      isEdit ? updateHelmRepository(existing!.id, payload) : createHelmRepository(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['helm-repositories'] })
      onClose()
    },
    onError: (e: any) => setError(e?.response?.data?.error || e.message),
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    mutate({
      name,
      url,
      username: username || undefined,
      password: password || undefined,
      tls_insecure: tlsInsecure,
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Display name</label>
        <input className="input w-full" placeholder="my-charts" value={name} onChange={(e) => setName(e.target.value)} required />
      </div>

      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Repository URL</label>
        <input
          className="input w-full font-mono text-xs"
          placeholder="https://charts.example.com"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          required
        />
        <p className="text-[11px] text-slate-600">KubePilot reads <span className="font-mono">index.yaml</span> at this URL.</p>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">Username <span className="text-slate-600">(optional)</span></label>
          <input className="input w-full" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" />
        </div>
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">
            Password / token {isEdit && <span className="text-slate-600">(leave blank to keep)</span>}
          </label>
          <input className="input w-full" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
        </div>
      </div>

      <label className="flex items-center gap-2 text-xs text-slate-400">
        <input type="checkbox" checked={tlsInsecure} onChange={(e) => setTlsInsecure(e.target.checked)} />
        Skip TLS verification (self-hosted ChartMuseum, internal Harbor…)
      </label>

      {error && (
        <div className="px-3 py-2 rounded bg-red-950/60 border border-red-900/50 text-red-400 text-xs">{error}</div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
        <button type="submit" className="btn btn-primary" disabled={isPending}>
          {isPending ? 'Saving...' : isEdit ? 'Save' : 'Add repository'}
        </button>
      </div>
    </form>
  )
}

function HelmRepoModal({ open, existing, onOpenChange }: { open: boolean; existing?: HelmRepository; onOpenChange: (o: boolean) => void }) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50 z-40" />
        <Dialog.Content className="fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-lg bg-surface-panel border border-surface-border rounded-xl shadow-2xl p-6 max-h-[90vh] overflow-y-auto">
          <div className="flex items-center justify-between mb-4">
            <Dialog.Title className="text-sm font-semibold text-slate-100">
              {existing ? 'Edit chart repository' : 'Add chart repository'}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button className="p-1 rounded hover:bg-surface-elevated text-slate-400 hover:text-slate-200">
                <X size={15} />
              </button>
            </Dialog.Close>
          </div>
          {open && <HelmRepoForm existing={existing} onClose={() => onOpenChange(false)} />}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function HelmRepositories() {
  const qc = useQueryClient()
  const { data: repos = [], isLoading } = useQuery({ queryKey: ['helm-repositories'], queryFn: getHelmRepositories })
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<HelmRepository | undefined>(undefined)
  const [testingId, setTestingId] = useState<string | null>(null)
  const [testResults, setTestResults] = useState<Record<string, HelmRepositoryTestResult>>({})

  function openAdd() {
    setEditing(undefined)
    setModalOpen(true)
  }

  async function handleTest(id: string) {
    setTestingId(id)
    try {
      const res = await testHelmRepository(id)
      setTestResults((prev) => ({ ...prev, [id]: res }))
    } finally {
      setTestingId(null)
    }
  }

  async function handleDelete(id: string) {
    if (!confirm('Delete this chart repository? Releases resolved through it will be re-resolved on the next check.')) return
    await deleteHelmRepository(id)
    await qc.invalidateQueries({ queryKey: ['helm-repositories'] })
  }

  return (
    <section className="mt-8">
      <div className="flex items-center justify-between mb-1">
        <h2 className="text-base font-semibold text-slate-100">Chart repositories</h2>
        <button className="btn btn-secondary" onClick={openAdd}>
          <Plus size={14} />
          Add repository
        </button>
      </div>
      <p className="text-xs text-slate-500 mb-4">
        A Helm release does not record where its chart came from. KubePilot looks the chart up in these repositories to
        detect newer versions — add your private ones here.
      </p>

      <HelmRepoModal open={modalOpen} existing={editing} onOpenChange={setModalOpen} />

      {isLoading ? (
        <div className="panel p-4 space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="skeleton h-10 w-full rounded" />)}
        </div>
      ) : repos.length === 0 ? (
        <div className="panel p-10 flex flex-col items-center gap-4">
          <Boxes size={28} className="text-slate-600" />
          <p className="text-sm text-slate-400">No chart repositories configured</p>
          <button className="btn btn-primary" onClick={openAdd}>
            <Plus size={14} />
            Add first repository
          </button>
        </div>
      ) : (
        <div className="panel overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-slate-500 border-b border-surface-border">
                <th className="px-4 py-2.5 font-medium">Name</th>
                <th className="px-4 py-2.5 font-medium">URL</th>
                <th className="px-4 py-2.5 font-medium">Auth</th>
                <th className="px-4 py-2.5 font-medium">Test</th>
                <th className="px-4 py-2.5 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {repos.map((r) => {
                const result = testResults[r.id]
                return (
                  <tr key={r.id} className="border-b border-surface-border/60 last:border-0 hover:bg-surface-elevated/40">
                    <td className="px-4 py-2.5 font-medium text-slate-200">
                      <div className="flex items-center gap-2">
                        {r.name}
                        {r.built_in && (
                          <span className="px-1.5 py-0.5 rounded text-[10px] bg-surface-elevated text-slate-500">built-in</span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-400">
                      <div className="flex items-center gap-1.5">
                        {r.url}
                        {r.tls_insecure && (
                          <span title="TLS verification disabled">
                            <ShieldAlert size={12} className="text-yellow-500" />
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-xs">
                      {r.has_credentials ? (
                        <span className="text-slate-300">{r.username || 'token'}</span>
                      ) : (
                        <span className="text-slate-600">anonymous</span>
                      )}
                    </td>
                    <td className="px-4 py-2.5">
                      {result ? (
                        <span className={clsx('inline-flex items-center gap-1 text-xs', result.ok ? 'text-green-400' : 'text-red-400')}>
                          {result.ok ? <CheckCircle size={12} /> : <XCircle size={12} />}
                          {result.ok ? `${result.charts} charts` : 'failed'}
                        </span>
                      ) : (
                        <span className="text-slate-600 text-xs">—</span>
                      )}
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-center justify-end gap-1.5">
                        <button
                          className="btn btn-secondary py-1 px-2 text-xs"
                          onClick={() => handleTest(r.id)}
                          disabled={testingId === r.id}
                          title={result?.message || 'Fetch index.yaml'}
                        >
                          <PlayCircle size={12} className={testingId === r.id ? 'animate-spin' : ''} />
                        </button>
                        <button
                          className="btn btn-secondary py-1 px-2 text-xs"
                          onClick={() => { setEditing(r); setModalOpen(true) }}
                          title="Edit"
                        >
                          <Pencil size={12} />
                        </button>
                        <button className="btn btn-danger py-1 px-2 text-xs" onClick={() => handleDelete(r.id)} title="Remove">
                          <Trash2 size={12} />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}

export function Registries() {
  const qc = useQueryClient()
  const { data: registries = [], isLoading } = useQuery({ queryKey: ['registries'], queryFn: getRegistries })
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ImageRegistry | undefined>(undefined)
  const [testingId, setTestingId] = useState<string | null>(null)
  const [testResults, setTestResults] = useState<Record<string, RegistryTestResult>>({})

  function openAdd() {
    setEditing(undefined)
    setModalOpen(true)
  }
  function openEdit(r: ImageRegistry) {
    setEditing(r)
    setModalOpen(true)
  }

  async function handleTest(id: string) {
    setTestingId(id)
    try {
      const res = await testRegistry(id)
      setTestResults((prev) => ({ ...prev, [id]: res }))
    } finally {
      setTestingId(null)
    }
  }

  async function handleDelete(id: string) {
    if (!confirm('Delete this registry? Images on its host will fall back to anonymous access.')) return
    await deleteRegistry(id)
    await qc.invalidateQueries({ queryKey: ['registries'] })
  }

  return (
    <div className="p-4 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-1">
        <h1 className="text-base font-semibold text-slate-100">Image registries</h1>
        <button className="btn btn-primary" onClick={openAdd}>
          <Plus size={14} />
          Add registry
        </button>
      </div>
      <p className="text-xs text-slate-500 mb-4">
        Configure credentials and TLS for private registries so KubePilot can read image tags and produce update findings.
      </p>

      <RegistryModal open={modalOpen} existing={editing} onOpenChange={setModalOpen} />

      {isLoading ? (
        <div className="panel p-4 space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="skeleton h-10 w-full rounded" />)}
        </div>
      ) : registries.length === 0 ? (
        <div className="panel p-12 flex flex-col items-center gap-4">
          <Boxes size={32} className="text-slate-600" />
          <div className="text-center">
            <p className="text-sm text-slate-400">No registries configured</p>
            <p className="text-xs text-slate-600 mt-1">
              Public images (Docker Hub, GHCR…) are scanned anonymously. Add a registry only for private or self-signed ones.
            </p>
          </div>
          <button className="btn btn-primary" onClick={openAdd}>
            <Plus size={14} />
            Add first registry
          </button>
        </div>
      ) : (
        <div className="panel overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-slate-500 border-b border-surface-border">
                <th className="px-4 py-2.5 font-medium">Name</th>
                <th className="px-4 py-2.5 font-medium">Host</th>
                <th className="px-4 py-2.5 font-medium">Type</th>
                <th className="px-4 py-2.5 font-medium">Auth</th>
                <th className="px-4 py-2.5 font-medium">Test</th>
                <th className="px-4 py-2.5 font-medium text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {registries.map((r) => {
                const result = testResults[r.id]
                return (
                  <tr key={r.id} className="border-b border-surface-border/60 last:border-0 hover:bg-surface-elevated/40">
                    <td className="px-4 py-2.5 font-medium text-slate-200">{r.name}</td>
                    <td className="px-4 py-2.5 font-mono text-xs text-slate-400">
                      <div className="flex items-center gap-1.5">
                        {r.host}
                        {r.tls_insecure && (
                          <span title="TLS verification disabled">
                            <ShieldAlert size={12} className="text-yellow-500" />
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-slate-400 capitalize">{r.type}</td>
                    <td className="px-4 py-2.5 text-xs">
                      {r.has_credentials ? (
                        <span className="text-slate-300">{r.username || 'token'}</span>
                      ) : (
                        <span className="text-slate-600">anonymous</span>
                      )}
                    </td>
                    <td className="px-4 py-2.5">
                      {result ? (
                        <span className={clsx('inline-flex items-center gap-1 text-xs', result.ok ? 'text-green-400' : 'text-red-400')}>
                          {result.ok ? <CheckCircle size={12} /> : <XCircle size={12} />}
                          {result.status_code || '—'}
                        </span>
                      ) : (
                        <span className="text-slate-600 text-xs">—</span>
                      )}
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-center justify-end gap-1.5">
                        <button
                          className="btn btn-secondary py-1 px-2 text-xs"
                          onClick={() => handleTest(r.id)}
                          disabled={testingId === r.id}
                          title={result?.message || 'Test connectivity'}
                        >
                          <PlayCircle size={12} className={testingId === r.id ? 'animate-spin' : ''} />
                        </button>
                        <button className="btn btn-secondary py-1 px-2 text-xs" onClick={() => openEdit(r)} title="Edit">
                          <Pencil size={12} />
                        </button>
                        <button className="btn btn-danger py-1 px-2 text-xs" onClick={() => handleDelete(r.id)} title="Remove">
                          <Trash2 size={12} />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      <HelmRepositories />
    </div>
  )
}
