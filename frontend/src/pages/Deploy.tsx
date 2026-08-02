import React, { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Eye, Rocket, ShieldCheck, CheckCircle2, RefreshCw, XCircle } from 'lucide-react'
import clsx from 'clsx'
import { applyManifest } from '../api/client'
import { useClusters } from '../hooks/useClusters'
import { useAuth } from '../contexts/AuthContext'
import type { ManifestApplyResult } from '../types'

const OPERATION_STYLES: Record<string, string> = {
  created: 'text-green-400 border-green-900/60 bg-green-950/40',
  updated: 'text-blue-400 border-blue-900/60 bg-blue-950/40',
  error: 'text-red-400 border-red-900/60 bg-red-950/40',
}

const OPERATION_ICONS: Record<string, React.ReactNode> = {
  created: <CheckCircle2 size={13} />,
  updated: <RefreshCw size={13} />,
  error: <XCircle size={13} />,
}

function ResultRow({ result }: { result: ManifestApplyResult }) {
  return (
    <div className="flex items-start gap-2 p-2 rounded bg-surface-elevated text-xs">
      <span className={clsx('flex items-center gap-1 px-1.5 py-0.5 rounded border font-medium flex-shrink-0', OPERATION_STYLES[result.operation])}>
        {OPERATION_ICONS[result.operation]}
        {result.operation}
      </span>
      <div className="min-w-0">
        <div className="font-mono text-slate-300">
          {result.kind}/{result.name}
          {result.namespace && <span className="text-slate-500"> — {result.namespace}</span>}
        </div>
        {result.error && <div className="text-red-400 mt-0.5">{result.error}</div>}
      </div>
    </div>
  )
}

export function Deploy() {
  const qc = useQueryClient()
  const { user } = useAuth()
  const canWrite = user?.role === 'admin' || user?.role === 'operator'
  const { data: clusters = [] } = useClusters()

  const [clusterId, setClusterId] = useState('')
  const [namespace, setNamespace] = useState('')
  const [manifest, setManifest] = useState('')
  const [force, setForce] = useState(false)
  const [busy, setBusy] = useState<'preview' | 'apply' | null>(null)
  const [results, setResults] = useState<ManifestApplyResult[] | null>(null)
  const [lastDryRun, setLastDryRun] = useState(false)
  const [error, setError] = useState('')

  if (!canWrite) {
    return (
      <div className="p-4 max-w-3xl mx-auto">
        <div className="panel p-12 flex flex-col items-center gap-3">
          <ShieldCheck size={28} className="text-slate-600" />
          <p className="text-sm text-slate-400">Deploying manifests is restricted to operators and administrators.</p>
        </div>
      </div>
    )
  }

  // Same gotcha as scale/restart and Helm upgrade/rollback: workloads and
  // Helm releases are only re-polled every 60s (no live Watch) — the apply
  // endpoint kicks an immediate sync, but it runs in the background after
  // the response comes back, so invalidate a couple more times over the
  // next few seconds to actually catch it.
  function refreshAfterApply() {
    const invalidate = () => {
      qc.invalidateQueries({ queryKey: ['workloads'] })
      qc.invalidateQueries({ queryKey: ['helm'] })
      qc.invalidateQueries({ queryKey: ['namespaces'] })
    }
    invalidate()
    setTimeout(invalidate, 1500)
    setTimeout(invalidate, 4000)
  }

  async function run(dryRun: boolean) {
    if (!clusterId) {
      setError('Select a cluster first')
      return
    }
    if (!manifest.trim()) {
      setError('Manifest is empty')
      return
    }
    if (!dryRun && !confirm('Apply this manifest to the live cluster? This modifies real resources.')) {
      return
    }

    setError('')
    setBusy(dryRun ? 'preview' : 'apply')
    try {
      const res = await applyManifest(clusterId, {
        manifest,
        namespace: namespace.trim() || undefined,
        dry_run: dryRun,
        force,
      })
      setResults(res.results)
      setLastDryRun(res.dry_run)
      if (!dryRun) refreshAfterApply()
    } catch (e: any) {
      setError(e?.response?.data?.error || e.message)
      setResults(null)
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-3 px-4 py-2 border-b border-surface-border bg-surface-panel">
        <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">Deploy a manifest</h2>
        <span className="text-xs text-slate-500">Server-side apply — one or more YAML documents</span>
      </div>

      <div className="flex-1 overflow-auto p-4">
        <div className="max-w-3xl mx-auto space-y-4">
          <div className="panel p-3 space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <label className="block text-xs text-slate-500">Cluster</label>
                <select className="input w-full text-xs" value={clusterId} onChange={(e) => setClusterId(e.target.value)}>
                  <option value="">Select a cluster…</option>
                  {clusters.map((c) => (
                    <option key={c.id} value={c.id}>{c.display_name || c.name}</option>
                  ))}
                </select>
              </div>
              <div className="space-y-1">
                <label className="block text-xs text-slate-500">
                  Default namespace <span className="text-slate-600">— used for resources with none set</span>
                </label>
                <input
                  className="input w-full text-xs font-mono"
                  placeholder="e.g. default"
                  value={namespace}
                  onChange={(e) => setNamespace(e.target.value)}
                />
              </div>
            </div>

            <div className="space-y-1">
              <label className="block text-xs text-slate-500">Manifest (YAML)</label>
              <textarea
                className="input w-full font-mono text-xs h-72 resize-y"
                placeholder={'apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: example\ndata:\n  key: value'}
                value={manifest}
                onChange={(e) => setManifest(e.target.value)}
                spellCheck={false}
              />
            </div>

            <label className="flex items-center gap-2 text-xs text-slate-500">
              <input type="checkbox" checked={force} onChange={(e) => setForce(e.target.checked)} />
              Force (resolve field-manager conflicts with other controllers)
            </label>

            {error && (
              <div className="px-3 py-2 rounded bg-red-950/60 border border-red-900/50 text-red-400 text-xs">{error}</div>
            )}

            <div className="flex gap-2">
              <button
                className="btn btn-secondary flex items-center gap-1.5"
                disabled={busy !== null}
                onClick={() => run(true)}
              >
                <Eye size={13} className={busy === 'preview' ? 'animate-pulse' : ''} />
                {busy === 'preview' ? 'Previewing…' : 'Preview (dry-run)'}
              </button>
              <button
                className="btn btn-primary flex items-center gap-1.5"
                disabled={busy !== null}
                onClick={() => run(false)}
              >
                <Rocket size={13} className={busy === 'apply' ? 'animate-pulse' : ''} />
                {busy === 'apply' ? 'Applying…' : 'Apply'}
              </button>
            </div>
          </div>

          {results && (
            <div className="panel p-3 space-y-2">
              <div className="text-xs font-medium text-slate-400 uppercase tracking-wider">
                Results {lastDryRun && <span className="text-yellow-500 normal-case">(dry-run — nothing was changed)</span>}
              </div>
              <div className="space-y-1.5">
                {results.map((r, i) => (
                  <ResultRow key={i} result={r} />
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
