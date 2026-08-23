import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as Dialog from '@radix-ui/react-dialog'
import { Plus, ShieldOff, Pencil, Trash2, X, Power } from 'lucide-react'
import clsx from 'clsx'
import {
  getExceptionRules,
  createExceptionRule,
  updateExceptionRule,
  deleteExceptionRule,
  getWorkloads,
  type ExceptionRulePayload,
} from '../api/client'
import { useClusters } from '../hooks/useClusters'
import { useAuth } from '../contexts/AuthContext'
import type { ExceptionRule, ExceptionRuleType } from '../types'

const RULE_TYPES: { value: ExceptionRuleType; label: string; desc: string }[] = [
  { value: 'suppress', label: 'Suppress', desc: 'Score forced to 0, severity to Info.' },
  { value: 'reduce_severity', label: 'Reduce severity', desc: 'Severity lowered one band, score unchanged.' },
  { value: 'accept_risk', label: 'Accept risk', desc: 'Finding status forced to Ignored.' },
]

const RULE_TYPE_STYLES: Record<ExceptionRuleType, string> = {
  suppress: 'bg-red-950/50 text-red-300 border-red-900/60',
  reduce_severity: 'bg-yellow-950/50 text-yellow-300 border-yellow-900/60',
  accept_risk: 'bg-blue-950/50 text-blue-300 border-blue-900/60',
}

type ScopeKind = 'global' | 'cluster' | 'namespace' | 'workload' | 'image_pattern'

// Mirrors the specificity precedence the scoring engine applies
// (internal/scoring.scopeSpecificity): workload > image_pattern >
// cluster+namespace > cluster > global.
function scopeKindOf(r: ExceptionRule): ScopeKind {
  if (r.workload_id) return 'workload'
  if (r.image_pattern) return 'image_pattern'
  if (r.cluster_id && r.namespace_name) return 'namespace'
  if (r.cluster_id) return 'cluster'
  return 'global'
}

function scopeLabel(r: ExceptionRule): string {
  switch (scopeKindOf(r)) {
    case 'workload':
      return `Workload: ${r.workload?.name ?? r.workload_id}`
    case 'image_pattern':
      return `Image: ${r.image_pattern}`
    case 'namespace':
      return `${r.cluster?.name ?? r.cluster_id} / ${r.namespace_name}`
    case 'cluster':
      return `Cluster: ${r.cluster?.name ?? r.cluster_id}`
    default:
      return 'Global'
  }
}

function RuleForm({ existing, onClose }: { existing?: ExceptionRule; onClose: () => void }) {
  const qc = useQueryClient()
  const isEdit = !!existing
  const [name, setName] = useState(existing?.name ?? '')
  const [ruleType, setRuleType] = useState<ExceptionRuleType>(existing?.rule_type ?? 'suppress')
  const [scopeKind, setScopeKind] = useState<ScopeKind>(existing ? scopeKindOf(existing) : 'global')
  const [clusterId, setClusterId] = useState(existing?.cluster_id ?? '')
  const [namespaceName, setNamespaceName] = useState(existing?.namespace_name ?? '')
  const [workloadId, setWorkloadId] = useState(existing?.workload_id ?? '')
  const [imagePattern, setImagePattern] = useState(existing?.image_pattern ?? '')
  const [findingKind, setFindingKind] = useState(existing?.finding_kind ?? '')
  const [reason, setReason] = useState(existing?.reason ?? '')
  const [expiresAt, setExpiresAt] = useState(existing?.expires_at ? existing.expires_at.slice(0, 10) : '')
  const [error, setError] = useState('')

  const { data: clusters = [] } = useClusters()
  const needsCluster = scopeKind === 'cluster' || scopeKind === 'namespace' || scopeKind === 'workload'

  const { data: workloadsResp } = useQuery({
    queryKey: ['workloads-for-exception-rule', clusterId],
    queryFn: () => getWorkloads({ cluster_id: clusterId, limit: 500 }),
    enabled: scopeKind === 'workload' && !!clusterId,
  })
  const workloads = workloadsResp?.data ?? []

  const { mutate, isPending } = useMutation({
    mutationFn: (payload: ExceptionRulePayload) =>
      isEdit ? updateExceptionRule(existing!.id, payload) : createExceptionRule(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['exception-rules'] })
      onClose()
    },
    onError: (e: any) => setError(e?.response?.data?.error || e.message),
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    if (scopeKind === 'workload' && !workloadId) {
      setError('Pick a workload.')
      return
    }
    if (scopeKind === 'image_pattern' && !imagePattern.trim()) {
      setError('Enter an image pattern.')
      return
    }
    mutate({
      name,
      rule_type: ruleType,
      cluster_id: needsCluster ? clusterId || undefined : undefined,
      namespace_name: scopeKind === 'namespace' ? namespaceName || undefined : undefined,
      workload_id: scopeKind === 'workload' ? workloadId || undefined : undefined,
      image_pattern: scopeKind === 'image_pattern' ? imagePattern.trim() : undefined,
      finding_kind: findingKind || undefined,
      reason,
      expires_at: expiresAt ? new Date(`${expiresAt}T00:00:00Z`).toISOString() : null,
      is_active: existing?.is_active ?? true,
    })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <label htmlFor="exc-rule-name" className="block text-xs font-medium text-slate-400">Name</label>
        <input id="exc-rule-name" className="input w-full" placeholder="Pinned legacy base image" value={name} onChange={(e) => setName(e.target.value)} required />
      </div>

      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Effect</label>
        <div className="grid grid-cols-3 gap-2">
          {RULE_TYPES.map((t) => (
            <button
              type="button"
              key={t.value}
              className={clsx(
                'text-left p-2 rounded border text-xs transition-colors',
                ruleType === t.value ? 'border-blue-600 bg-blue-950/30' : 'border-surface-border bg-surface-elevated/40 hover:border-slate-600'
              )}
              onClick={() => setRuleType(t.value)}
            >
              <div className="font-medium text-slate-200">{t.label}</div>
              <div className="text-slate-500 mt-0.5 leading-snug">{t.desc}</div>
            </button>
          ))}
        </div>
      </div>

      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Scope</label>
        <select
          className="input w-full"
          value={scopeKind}
          onChange={(e) => setScopeKind(e.target.value as ScopeKind)}
        >
          <option value="global">Global — every finding</option>
          <option value="cluster">One cluster</option>
          <option value="namespace">One cluster + namespace</option>
          <option value="workload">One workload</option>
          <option value="image_pattern">Image pattern (glob)</option>
        </select>
      </div>

      {needsCluster && (
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">Cluster</label>
          <select
            className="input w-full"
            value={clusterId}
            onChange={(e) => { setClusterId(e.target.value); setWorkloadId('') }}
            required
          >
            <option value="">Select a cluster…</option>
            {clusters.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
        </div>
      )}

      {scopeKind === 'namespace' && (
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">Namespace</label>
          <input
            className="input w-full font-mono text-xs"
            placeholder="payments"
            value={namespaceName}
            onChange={(e) => setNamespaceName(e.target.value)}
            required
          />
        </div>
      )}

      {scopeKind === 'workload' && (
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">Workload</label>
          <select
            className="input w-full"
            value={workloadId}
            onChange={(e) => setWorkloadId(e.target.value)}
            required
            disabled={!clusterId}
          >
            <option value="">{clusterId ? 'Select a workload…' : 'Pick a cluster first'}</option>
            {workloads.map((w) => (
              <option key={w.id} value={w.id}>{w.name} ({w.namespace_name ?? w.namespace_id})</option>
            ))}
          </select>
        </div>
      )}

      {scopeKind === 'image_pattern' && (
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">Image pattern</label>
          <input
            className="input w-full font-mono text-xs"
            placeholder="myregistry.io/team/*:latest"
            value={imagePattern}
            onChange={(e) => setImagePattern(e.target.value)}
            required
          />
          <p className="text-[11px] text-slate-600">
            Matched against <span className="font-mono">registry/repository:tag</span> — <span className="font-mono">*</span> matches within one path segment.
          </p>
        </div>
      )}

      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Finding kind <span className="text-slate-600">(optional)</span></label>
        <select className="input w-full" value={findingKind} onChange={(e) => setFindingKind(e.target.value)}>
          <option value="">Any</option>
          <option value="image">Image</option>
          <option value="helm">Helm</option>
        </select>
      </div>

      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Reason</label>
        <textarea
          className="input w-full"
          rows={2}
          placeholder="Why this exception exists — shown on the applied badge"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          required
        />
      </div>

      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Expires <span className="text-slate-600">(optional — permanent if blank)</span></label>
        <input className="input w-full" type="date" value={expiresAt} onChange={(e) => setExpiresAt(e.target.value)} />
      </div>

      {error && (
        <div className="px-3 py-2 rounded bg-red-950/60 border border-red-900/50 text-red-400 text-xs">{error}</div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
        <button type="submit" className="btn btn-primary" disabled={isPending}>
          {isPending ? 'Saving...' : isEdit ? 'Save' : 'Add rule'}
        </button>
      </div>
    </form>
  )
}

function RuleModal({ open, existing, onOpenChange }: { open: boolean; existing?: ExceptionRule; onOpenChange: (o: boolean) => void }) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50 z-40" />
        <Dialog.Content className="fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-lg bg-surface-panel border border-surface-border rounded-xl shadow-2xl p-6 max-h-[90vh] overflow-y-auto">
          <div className="flex items-center justify-between mb-4">
            <Dialog.Title className="text-sm font-semibold text-slate-100">
              {existing ? 'Edit exception rule' : 'New exception rule'}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button className="p-1 rounded hover:bg-surface-elevated text-slate-400 hover:text-slate-200">
                <X size={15} />
              </button>
            </Dialog.Close>
          </div>
          {open && <RuleForm existing={existing} onClose={() => onOpenChange(false)} />}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

export function ExceptionRules() {
  const qc = useQueryClient()
  const { user } = useAuth()
  const canWrite = user?.role === 'admin' || user?.role === 'operator'
  const { data: rules = [], isLoading } = useQuery({ queryKey: ['exception-rules'], queryFn: getExceptionRules })
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ExceptionRule | undefined>(undefined)

  function openAdd() {
    setEditing(undefined)
    setModalOpen(true)
  }
  function openEdit(r: ExceptionRule) {
    setEditing(r)
    setModalOpen(true)
  }

  async function handleDelete(r: ExceptionRule) {
    if (!confirm(`Delete "${r.name}"? Matching findings will be scored normally again.`)) return
    await deleteExceptionRule(r.id)
    await qc.invalidateQueries({ queryKey: ['exception-rules'] })
  }

  async function toggleActive(r: ExceptionRule) {
    await updateExceptionRule(r.id, {
      name: r.name,
      rule_type: r.rule_type,
      cluster_id: r.cluster_id,
      namespace_name: r.namespace_name,
      workload_id: r.workload_id,
      finding_kind: r.finding_kind,
      image_pattern: r.image_pattern,
      reason: r.reason,
      expires_at: r.expires_at ?? null,
      is_active: !r.is_active,
    })
    await qc.invalidateQueries({ queryKey: ['exception-rules'] })
  }

  const now = Date.now()

  return (
    <div className="p-4 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-1">
        <h1 className="text-base font-semibold text-slate-100">Exception rules</h1>
        {canWrite && (
          <button className="btn btn-primary" onClick={openAdd}>
            <Plus size={14} />
            New rule
          </button>
        )}
      </div>
      <p className="text-xs text-slate-500 mb-4">
        Change how the scoring engine treats findings matching a scope — suppress, downgrade, or mark risk accepted.
        Useful when you're stuck on a version for a known reason and don't need KubePilot to keep flagging it.
      </p>

      {canWrite && <RuleModal open={modalOpen} existing={editing} onOpenChange={setModalOpen} />}

      {isLoading ? (
        <div className="panel p-4 space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="skeleton h-10 w-full rounded" />)}
        </div>
      ) : rules.length === 0 ? (
        <div className="panel p-12 flex flex-col items-center gap-4">
          <ShieldOff size={32} className="text-slate-600" />
          <div className="text-center">
            <p className="text-sm text-slate-400">No exception rules</p>
            <p className="text-xs text-slate-600 mt-1">Every finding is scored and surfaced normally.</p>
          </div>
          {canWrite && (
            <button className="btn btn-primary" onClick={openAdd}>
              <Plus size={14} />
              Add first rule
            </button>
          )}
        </div>
      ) : (
        <div className="panel overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-slate-500 border-b border-surface-border">
                <th className="px-4 py-2.5 font-medium">Name</th>
                <th className="px-4 py-2.5 font-medium">Effect</th>
                <th className="px-4 py-2.5 font-medium">Scope</th>
                <th className="px-4 py-2.5 font-medium">Reason</th>
                <th className="px-4 py-2.5 font-medium">Expires</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
                {canWrite && <th className="px-4 py-2.5 font-medium text-right">Actions</th>}
              </tr>
            </thead>
            <tbody>
              {rules.map((r) => {
                const expired = r.expires_at ? new Date(r.expires_at).getTime() < now : false
                return (
                  <tr key={r.id} className="border-b border-surface-border/60 last:border-0 hover:bg-surface-elevated/40">
                    <td className="px-4 py-2.5 font-medium text-slate-200">{r.name}</td>
                    <td className="px-4 py-2.5">
                      <span className={clsx('px-1.5 py-0.5 rounded text-[10px] border whitespace-nowrap', RULE_TYPE_STYLES[r.rule_type])}>
                        {r.rule_type.replace('_', ' ')}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-xs text-slate-400">{scopeLabel(r)}</td>
                    <td className="px-4 py-2.5 text-xs text-slate-400 max-w-xs truncate" title={r.reason}>{r.reason}</td>
                    <td className="px-4 py-2.5 text-xs text-slate-500 whitespace-nowrap">
                      {r.expires_at ? new Date(r.expires_at).toLocaleDateString() : 'never'}
                    </td>
                    <td className="px-4 py-2.5 text-xs">
                      {!r.is_active ? (
                        <span className="text-slate-600">paused</span>
                      ) : expired ? (
                        <span className="text-slate-600">expired</span>
                      ) : (
                        <span className="text-green-400">active</span>
                      )}
                    </td>
                    {canWrite && (
                      <td className="px-4 py-2.5">
                        <div className="flex items-center justify-end gap-1.5">
                          <button
                            className="btn btn-secondary py-1 px-2 text-xs"
                            onClick={() => toggleActive(r)}
                            title={r.is_active ? 'Pause' : 'Resume'}
                          >
                            <Power size={12} />
                          </button>
                          <button className="btn btn-secondary py-1 px-2 text-xs" onClick={() => openEdit(r)} title="Edit">
                            <Pencil size={12} />
                          </button>
                          <button className="btn btn-danger py-1 px-2 text-xs" onClick={() => handleDelete(r)} title="Delete">
                            <Trash2 size={12} />
                          </button>
                        </div>
                      </td>
                    )}
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
