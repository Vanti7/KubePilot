import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import clsx from 'clsx'
import { DataTable, Column } from '../components/DataTable'
import { SlideOver } from '../components/SlideOver'
import { getActionLogs } from '../api/client'
import type { ActionLog } from '../types'
import { formatRelative } from '../utils/formatting'

// Kept in sync by hand with the models.ActionLog* / RecordAction call sites
// in backend/internal/api/handlers — there is no list endpoint for the
// distinct values actually in use.
const ACTIONS = [
  'create', 'update', 'delete', 'update_status', 'resolve', 'ignore', 'sync',
  'scale', 'restart', 'helm_upgrade', 'helm_rollback', 'deploy_manifest', 'update_image',
]
const ENTITY_TYPES = ['workload', 'helm_release', 'cluster']
const STATUSES = ['success', 'failure']

const ACTION_LABELS: Record<string, string> = {
  create: 'Create',
  update: 'Update',
  delete: 'Delete',
  update_status: 'Status change',
  resolve: 'Resolve',
  ignore: 'Ignore',
  sync: 'Sync',
  scale: 'Scale',
  restart: 'Restart',
  helm_upgrade: 'Helm upgrade',
  helm_rollback: 'Helm rollback',
  deploy_manifest: 'Deploy manifest',
  update_image: 'Image update',
}

const PAGE_SIZE = 50

export function History() {
  const [page, setPage] = useState(0)
  const [action, setAction] = useState('')
  const [entityType, setEntityType] = useState('')
  const [status, setStatus] = useState('')
  const [active, setActive] = useState<ActionLog | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['action-logs', page, action, entityType, status],
    queryFn: () =>
      getActionLogs({
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
        action: action || undefined,
        entity_type: entityType || undefined,
        status: status || undefined,
      }),
  })

  const logs = data?.data ?? []
  const total = data?.total ?? 0

  function resetToFirstPage<T>(setter: (v: T) => void) {
    return (v: T) => {
      setter(v)
      setPage(0)
    }
  }

  const columns: Column<ActionLog>[] = [
    {
      key: 'created_at',
      header: 'When',
      width: '15%',
      render: (r) => (
        <span title={new Date(r.created_at).toLocaleString()} className="text-slate-400">
          {formatRelative(r.created_at)}
        </span>
      ),
    },
    {
      key: 'user',
      header: 'User',
      width: '18%',
      render: (r) => r.user?.name || r.user?.email || <span className="text-slate-600">system</span>,
    },
    {
      key: 'action',
      header: 'Action',
      width: '18%',
      render: (r) => (
        <span className="px-1.5 py-0.5 rounded text-[10px] bg-surface-elevated text-slate-300 border border-surface-border whitespace-nowrap">
          {ACTION_LABELS[r.action] ?? r.action}
        </span>
      ),
    },
    {
      key: 'entity',
      header: 'Entity',
      render: (r) => (
        <span className="font-mono text-xs text-slate-500">
          {r.entity_type} · {r.entity_id.slice(0, 8)}
        </span>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      width: '10%',
      render: (r) => (
        <span
          className={clsx(
            'px-1.5 py-0.5 rounded text-[10px] border',
            r.status === 'success'
              ? 'bg-green-950/60 text-green-400 border-green-900/50'
              : 'bg-red-950/60 text-red-400 border-red-900/50'
          )}
        >
          {r.status}
        </span>
      ),
    },
  ]

  return (
    <div className="p-4 max-w-6xl mx-auto">
      <div className="flex items-center justify-between mb-1">
        <h1 className="text-base font-semibold text-slate-100">History</h1>
      </div>
      <p className="text-xs text-slate-500 mb-4">
        Audit trail of every write action KubePilot has taken — scale/restart, Helm upgrade/rollback, manifest
        deploys, and image bumps triggered from a finding.
      </p>

      <div className="flex items-center gap-2 mb-3">
        <select className="input py-1 text-xs" value={action} onChange={(e) => resetToFirstPage(setAction)(e.target.value)}>
          <option value="">Any action</option>
          {ACTIONS.map((a) => (
            <option key={a} value={a}>{ACTION_LABELS[a] ?? a}</option>
          ))}
        </select>
        <select className="input py-1 text-xs" value={entityType} onChange={(e) => resetToFirstPage(setEntityType)(e.target.value)}>
          <option value="">Any entity</option>
          {ENTITY_TYPES.map((t) => (
            <option key={t} value={t}>{t}</option>
          ))}
        </select>
        <select className="input py-1 text-xs" value={status} onChange={(e) => resetToFirstPage(setStatus)(e.target.value)}>
          <option value="">Any status</option>
          {STATUSES.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>
      </div>

      <div className="panel overflow-hidden">
        <DataTable
          columns={columns}
          data={logs}
          loading={isLoading}
          onRowClick={(r) => setActive(r)}
          rowKey={(r) => r.id}
          emptyMessage="No actions recorded yet"
        />
      </div>

      {total > PAGE_SIZE && (
        <div className="flex items-center justify-between px-1 py-3">
          <span className="text-xs text-slate-500">
            {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, total)} of {total}
          </span>
          <div className="flex items-center gap-2">
            <button className="btn btn-secondary py-1 px-2 text-xs" disabled={page === 0} onClick={() => setPage(page - 1)}>
              Previous
            </button>
            <button
              className="btn btn-secondary py-1 px-2 text-xs"
              disabled={(page + 1) * PAGE_SIZE >= total}
              onClick={() => setPage(page + 1)}
            >
              Next
            </button>
          </div>
        </div>
      )}

      <SlideOver open={!!active} onClose={() => setActive(null)} title="Action detail">
        {active && (
          <div className="p-4 space-y-4 text-sm">
            <div>
              <span className="text-xs text-slate-500 block mb-1">When</span>
              {new Date(active.created_at).toLocaleString()}
            </div>
            <div>
              <span className="text-xs text-slate-500 block mb-1">User</span>
              {active.user?.name || active.user?.email || 'system'}
            </div>
            <div>
              <span className="text-xs text-slate-500 block mb-1">Action</span>
              {ACTION_LABELS[active.action] ?? active.action}
            </div>
            <div>
              <span className="text-xs text-slate-500 block mb-1">Entity</span>
              <span className="font-mono text-xs">{active.entity_type} / {active.entity_id}</span>
            </div>
            <div>
              <span className="text-xs text-slate-500 block mb-1">Status</span>
              <span className={active.status === 'success' ? 'text-green-400' : 'text-red-400'}>{active.status}</span>
            </div>
            {active.ip_address && (
              <div>
                <span className="text-xs text-slate-500 block mb-1">Client IP</span>
                <span className="font-mono text-xs">{active.ip_address}</span>
              </div>
            )}
            {active.details && Object.keys(active.details).length > 0 && (
              <div>
                <span className="text-xs text-slate-500 block mb-1">Details</span>
                <pre className="bg-surface-elevated rounded p-3 text-xs overflow-x-auto whitespace-pre-wrap break-all">
                  {JSON.stringify(active.details, null, 2)}
                </pre>
              </div>
            )}
          </div>
        )}
      </SlideOver>
    </div>
  )
}
