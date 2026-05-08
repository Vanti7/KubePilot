import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as Dialog from '@radix-ui/react-dialog'
import {
  Plus,
  Plug,
  Trash2,
  PlayCircle,
  CheckCircle,
  XCircle,
  Clock,
  X,
} from 'lucide-react'
import clsx from 'clsx'
import { getIntegrations, createIntegration, testIntegration, deleteIntegration } from '../api/client'
import type { IntegrationAccount } from '../types'
import { formatRelative } from '../utils/formatting'

const INTEGRATION_TYPES = [
  { value: 'slack', label: 'Slack' },
  { value: 'pagerduty', label: 'PagerDuty' },
  { value: 'jira', label: 'Jira' },
  { value: 'github', label: 'GitHub' },
  { value: 'gitlab', label: 'GitLab' },
  { value: 'webhook', label: 'Webhook' },
  { value: 'prometheus', label: 'Prometheus' },
  { value: 'datadog', label: 'Datadog' },
]

const TYPE_ICONS: Record<string, React.ReactNode> = {
  slack: <span className="text-lg">💬</span>,
  pagerduty: <span className="text-lg">🔔</span>,
  jira: <span className="text-lg">🎯</span>,
  github: <span className="text-lg">🐙</span>,
  gitlab: <span className="text-lg">🦊</span>,
  webhook: <span className="text-lg">🔗</span>,
  prometheus: <span className="text-lg">📊</span>,
  datadog: <span className="text-lg">🐶</span>,
}

function IntegrationCard({
  integration,
  onTest,
  onDelete,
  testing,
}: {
  integration: IntegrationAccount
  onTest: () => void
  onDelete: () => void
  testing: boolean
}) {
  const isOk = integration.last_test_status === 'ok' || integration.last_test_status === 'success'
  const isFail = integration.last_test_status === 'failed' || integration.last_test_status === 'error'

  return (
    <div className="panel p-4 flex flex-col gap-3">
      <div className="flex items-start gap-3">
        <div className="w-9 h-9 rounded-lg bg-surface-elevated border border-surface-border flex items-center justify-center flex-shrink-0">
          {TYPE_ICONS[integration.integration_type] || <Plug size={18} className="text-slate-400" />}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-semibold text-slate-200 truncate">{integration.name}</h3>
            <span
              className={clsx(
                'w-2 h-2 rounded-full flex-shrink-0',
                integration.enabled ? 'bg-green-400' : 'bg-slate-600'
              )}
            />
          </div>
          <p className="text-xs text-slate-500 capitalize">{integration.integration_type}</p>
        </div>
      </div>

      {/* Status */}
      <div className="flex items-center gap-1.5 text-xs">
        {isOk && <CheckCircle size={13} className="text-green-400" />}
        {isFail && <XCircle size={13} className="text-red-400" />}
        {!isOk && !isFail && <Clock size={13} className="text-slate-500" />}
        <span className={clsx(isOk ? 'text-green-400' : isFail ? 'text-red-400' : 'text-slate-500')}>
          {isOk ? 'Connected' : isFail ? 'Failed' : 'Not tested'}
        </span>
        {integration.last_tested_at && (
          <span className="text-slate-600 ml-1">
            — {formatRelative(integration.last_tested_at)}
          </span>
        )}
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2 pt-1 border-t border-surface-border">
        <button
          className="btn btn-secondary py-1 text-xs flex-1 justify-center"
          onClick={onTest}
          disabled={testing}
        >
          <PlayCircle size={13} className={testing ? 'animate-spin' : ''} />
          {testing ? 'Testing...' : 'Test'}
        </button>
        <button
          className="btn btn-danger py-1 text-xs"
          onClick={onDelete}
        >
          <Trash2 size={13} />
        </button>
      </div>
    </div>
  )
}

function AddIntegrationModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [type, setType] = useState('slack')
  const [url, setUrl] = useState('')
  const [credRef, setCredRef] = useState('')
  const [error, setError] = useState('')

  const { mutate, isPending } = useMutation({
    mutationFn: createIntegration,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['integrations'] })
      onClose()
    },
    onError: (e: Error) => setError(e.message),
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    mutate({ name, integration_type: type, url: url || undefined, credentials_ref: credRef || undefined })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Name</label>
        <input
          className="input w-full"
          placeholder="My Slack workspace"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
      </div>

      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Type</label>
        <select
          className="input w-full"
          value={type}
          onChange={(e) => setType(e.target.value)}
        >
          {INTEGRATION_TYPES.map((t) => (
            <option key={t.value} value={t.value}>{t.label}</option>
          ))}
        </select>
      </div>

      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">URL <span className="text-slate-600">(optional)</span></label>
        <input
          className="input w-full"
          placeholder="https://hooks.example.com/..."
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          type="url"
        />
      </div>

      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Credentials ref <span className="text-slate-600">(secret name)</span></label>
        <input
          className="input w-full"
          placeholder="k8s-secret-name or vault path"
          value={credRef}
          onChange={(e) => setCredRef(e.target.value)}
        />
      </div>

      {error && (
        <div className="px-3 py-2 rounded bg-red-950/60 border border-red-900/50 text-red-400 text-xs">
          {error}
        </div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <button type="button" className="btn btn-secondary" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn btn-primary" disabled={isPending}>
          {isPending ? 'Adding...' : 'Add integration'}
        </button>
      </div>
    </form>
  )
}

export function Integrations() {
  const qc = useQueryClient()
  const [addOpen, setAddOpen] = useState(false)
  const [testingId, setTestingId] = useState<string | null>(null)

  const { data: integrations = [], isLoading } = useQuery({
    queryKey: ['integrations'],
    queryFn: getIntegrations,
  })

  async function handleTest(id: string) {
    setTestingId(id)
    try {
      await testIntegration(id)
      await qc.invalidateQueries({ queryKey: ['integrations'] })
    } finally {
      setTestingId(null)
    }
  }

  async function handleDelete(id: string) {
    await deleteIntegration(id)
    await qc.invalidateQueries({ queryKey: ['integrations'] })
  }

  return (
    <div className="p-4 max-w-5xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-base font-semibold text-slate-100">Integrations</h1>
        <Dialog.Root open={addOpen} onOpenChange={setAddOpen}>
          <Dialog.Trigger asChild>
            <button className="btn btn-primary">
              <Plus size={14} />
              Add integration
            </button>
          </Dialog.Trigger>

          <Dialog.Portal>
            <Dialog.Overlay className="fixed inset-0 bg-black/50 z-40" />
            <Dialog.Content className="fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-md bg-surface-panel border border-surface-border rounded-xl shadow-2xl p-6">
              <div className="flex items-center justify-between mb-4">
                <Dialog.Title className="text-sm font-semibold text-slate-100">
                  Add Integration
                </Dialog.Title>
                <Dialog.Close asChild>
                  <button className="p-1 rounded hover:bg-surface-elevated text-slate-400 hover:text-slate-200">
                    <X size={15} />
                  </button>
                </Dialog.Close>
              </div>
              <AddIntegrationModal onClose={() => setAddOpen(false)} />
            </Dialog.Content>
          </Dialog.Portal>
        </Dialog.Root>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="panel p-4 space-y-3">
              <div className="skeleton h-9 w-9 rounded-lg" />
              <div className="skeleton h-4 w-32 rounded" />
              <div className="skeleton h-3 w-20 rounded" />
            </div>
          ))}
        </div>
      ) : integrations.length === 0 ? (
        <div className="panel p-12 flex flex-col items-center gap-4">
          <Plug size={32} className="text-slate-600" />
          <div className="text-center">
            <p className="text-sm text-slate-400">No integrations configured</p>
            <p className="text-xs text-slate-600 mt-1">Connect KubePilot to Slack, PagerDuty, Jira and more</p>
          </div>
          <button className="btn btn-primary" onClick={() => setAddOpen(true)}>
            <Plus size={14} />
            Add first integration
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-3 gap-4">
          {integrations.map((integration) => (
            <IntegrationCard
              key={integration.id}
              integration={integration}
              onTest={() => handleTest(integration.id)}
              onDelete={() => handleDelete(integration.id)}
              testing={testingId === integration.id}
            />
          ))}
        </div>
      )}
    </div>
  )
}
