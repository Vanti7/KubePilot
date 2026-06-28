import React, { useState, useEffect } from 'react'
import { useSearchParams, Link } from 'react-router-dom'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Filter,
  ChevronDown,
  ExternalLink,
  MoreHorizontal,
  Check,
  X,
  Container,
  Download,
} from 'lucide-react'
import clsx from 'clsx'
import { DataTable, Column } from '../components/DataTable'
import { SeverityBadge } from '../components/SeverityBadge'
import { StatusBadge } from '../components/StatusBadge'
import { SlideOver } from '../components/SlideOver'
import { FindingDetail } from '../components/FindingDetail'
import { useFindingsFilter, useFindings, type FindingsFilterState } from '../hooks/useFindings'
import { useClusters } from '../hooks/useClusters'
import { updateFindingStatus, getRegistries, exportFindingsCsv } from '../api/client'
import type { UpdateFinding, Severity, FindingStatus, UpdateType } from '../types'
import { formatAge, scoreToColor, scoreToBg, updateTypeLabel } from '../utils/formatting'

const SEVERITIES: Severity[] = ['critical', 'high', 'medium', 'low', 'info']
const STATUSES: FindingStatus[] = ['open', 'planned', 'ignored', 'approved', 'blocked', 'resolved']
const UPDATE_TYPES: UpdateType[] = ['major', 'minor', 'patch', 'unknown']

const SEV_PILL: Record<Severity, string> = {
  critical: 'bg-red-950/80 text-red-400 border-red-900/60',
  high: 'bg-orange-950/80 text-orange-400 border-orange-900/60',
  medium: 'bg-yellow-950/80 text-yellow-400 border-yellow-900/60',
  low: 'bg-blue-950/80 text-blue-400 border-blue-900/60',
  info: 'bg-slate-800 text-slate-400 border-slate-700',
}

const UPDATE_TYPE_STYLES: Record<UpdateType, string> = {
  major: 'bg-red-950/60 text-red-400 border border-red-900/50',
  minor: 'bg-yellow-950/60 text-yellow-400 border border-yellow-900/50',
  patch: 'bg-green-950/60 text-green-400 border border-green-900/50',
  unknown: 'bg-slate-800 text-slate-400 border border-slate-700',
}

function MultiSelectPill<T extends string>({
  label,
  options,
  selected,
  onChange,
  renderOption,
}: {
  label: string
  options: T[]
  selected: T[]
  onChange: (vals: T[]) => void
  renderOption?: (v: T) => React.ReactNode
}) {
  function toggle(v: T) {
    onChange(selected.includes(v) ? selected.filter((x) => x !== v) : [...selected, v])
  }

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button
          className={clsx(
            'inline-flex items-center gap-1.5 px-2.5 py-1 rounded border text-xs transition-colors',
            selected.length > 0
              ? 'bg-blue-950/60 border-blue-800 text-blue-300'
              : 'bg-surface-elevated border-surface-border text-slate-400 hover:text-slate-200'
          )}
        >
          <Filter size={11} />
          {label}
          {selected.length > 0 && (
            <span className="bg-blue-700 text-white rounded-full px-1.5 text-xs">{selected.length}</span>
          )}
          <ChevronDown size={11} />
        </button>
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          className="z-50 min-w-[160px] bg-surface-elevated border border-surface-border rounded-lg shadow-xl py-1 text-sm"
          sideOffset={4}
        >
          {options.map((opt) => (
            <DropdownMenu.Item
              key={opt}
              className="flex items-center gap-2 px-3 py-1.5 cursor-pointer hover:bg-surface-panel outline-none text-slate-300"
              onSelect={(e) => {
                e.preventDefault()
                toggle(opt)
              }}
            >
              <span className="w-4 flex-shrink-0">
                {selected.includes(opt) && <Check size={12} className="text-blue-400" />}
              </span>
              {renderOption ? renderOption(opt) : opt}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

export function Updates({ initialFilter }: { initialFilter?: Partial<FindingsFilterState> } = {}) {
  const [searchParams] = useSearchParams()
  const { filter, setFilter, apiFilter } = useFindingsFilter(initialFilter)
  const { data, isLoading } = useFindings(apiFilter)
  const { data: clusters = [] } = useClusters()
  const { data: registries = [] } = useQuery({ queryKey: ['registries'], queryFn: getRegistries })
  const qc = useQueryClient()

  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [activeFinding, setActiveFinding] = useState<UpdateFinding | null>(null)

  // Handle ?finding= query param
  useEffect(() => {
    const findingId = searchParams.get('finding')
    if (findingId && data?.data) {
      const f = data.data.find((x) => x.id === findingId)
      if (f) setActiveFinding(f)
    }
  }, [searchParams, data])

  function handleSort(key: string) {
    if (filter.sortBy === key) {
      setFilter({ ...filter, sortDir: filter.sortDir === 'desc' ? 'asc' : 'desc' })
    } else {
      setFilter({ ...filter, sortBy: key, sortDir: 'desc' })
    }
  }

  function handleSelectRow(id: string, checked: boolean) {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      checked ? next.add(id) : next.delete(id)
      return next
    })
  }

  function handleSelectAll(checked: boolean) {
    if (checked) {
      setSelectedIds(new Set(data?.data.map((f) => f.id) ?? []))
    } else {
      setSelectedIds(new Set())
    }
  }

  async function bulkUpdateStatus(status: FindingStatus) {
    await Promise.all(
      Array.from(selectedIds).map((id) => updateFindingStatus(id, status))
    )
    await qc.invalidateQueries({ queryKey: ['findings'] })
    await qc.invalidateQueries({ queryKey: ['overview'] })
    setSelectedIds(new Set())
  }

  const findings = data?.data ?? []
  const total = data?.total ?? 0

  const columns: Column<UpdateFinding>[] = [
    {
      key: 'severity',
      header: 'Sev',
      width: '70px',
      sortable: true,
      render: (f) =>
        f.risk_score ? (
          <SeverityBadge severity={f.risk_score.severity} />
        ) : (
          <span className="text-slate-600 text-xs">—</span>
        ),
    },
    {
      key: 'resource',
      header: 'Resource',
      render: (f) => (
        <div className="flex items-center gap-1.5 flex-wrap">
          <span className="text-slate-200 text-xs font-medium">{f.workload_name || f.target_id}</span>
          <span className="px-1 py-0.5 text-xs rounded bg-surface-elevated text-slate-500 border border-surface-border">
            {f.target_kind}
          </span>
        </div>
      ),
    },
    {
      key: 'update',
      header: 'Update',
      render: (f) => (
        <div className="flex items-center gap-1.5">
          <span className="font-mono text-xs text-slate-400">
            {f.current_version} <span className="text-slate-600">→</span>{' '}
            <span className="text-green-400">{f.latest_version}</span>
          </span>
          <span className={clsx('px-1 py-0.5 text-xs rounded font-medium', UPDATE_TYPE_STYLES[f.update_type])}>
            {updateTypeLabel(f.update_type)}
          </span>
          {f.is_breaking && (
            <span className="px-1 py-0.5 text-xs rounded bg-red-950/80 text-red-400 border border-red-900/60 font-medium">
              BREAK
            </span>
          )}
        </div>
      ),
    },
    {
      key: 'score',
      header: 'Score',
      width: '90px',
      sortable: true,
      render: (f) => {
        const score = f.risk_score?.score
        return (
          <div className="flex items-center gap-1.5">
            <div className="w-12 h-1.5 bg-surface-elevated rounded-full overflow-hidden">
              <div
                className={clsx('h-full rounded-full', scoreToBg(score ?? 0))}
                style={{ width: `${score ?? 0}%` }}
              />
            </div>
            <span className={clsx('font-mono text-xs', scoreToColor(score ?? 0))}>
              {score ?? '—'}
            </span>
          </div>
        )
      },
    },
    {
      key: 'cluster',
      header: 'Cluster',
      render: (f) => (
        <div className="flex flex-col gap-0.5">
          <span className="text-xs font-mono text-slate-300">{f.cluster_name || '—'}</span>
          {f.namespace_name && (
            <span className="text-xs text-slate-500">{f.namespace_name}</span>
          )}
        </div>
      ),
    },
    {
      key: 'age',
      header: 'Age',
      width: '55px',
      sortable: true,
      render: (f) => (
        <span className="text-xs text-slate-500 font-mono">{formatAge(f.first_detected_at)}</span>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      width: '100px',
      render: (f) => <StatusBadge status={f.status} />,
    },
    {
      key: 'actions',
      header: '',
      width: '36px',
      render: (f) => (
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              className="p-1 rounded hover:bg-surface-elevated text-slate-500 hover:text-slate-300 transition-colors"
              onClick={(e) => e.stopPropagation()}
            >
              <MoreHorizontal size={15} />
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content
              className="z-50 min-w-[180px] bg-surface-elevated border border-surface-border rounded-lg shadow-xl py-1 text-sm"
              sideOffset={4}
              align="end"
            >
              {f.changelog_url && (
                <DropdownMenu.Item
                  className="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-surface-panel outline-none text-slate-300"
                  onSelect={() => window.open(f.changelog_url, '_blank')}
                >
                  <ExternalLink size={13} />
                  View changelog
                </DropdownMenu.Item>
              )}
              <DropdownMenu.Item
                className="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-surface-panel outline-none text-slate-300"
                onSelect={() => updateFindingStatus(f.id, 'planned').then(() => qc.invalidateQueries({ queryKey: ['findings'] }))}
              >
                Mark as planned
              </DropdownMenu.Item>
              <DropdownMenu.Item
                className="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-surface-panel outline-none text-slate-300"
                onSelect={() => updateFindingStatus(f.id, 'ignored').then(() => qc.invalidateQueries({ queryKey: ['findings'] }))}
              >
                Ignore
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      ),
    },
  ]

  return (
    <div className="flex flex-col h-full">
      {/* Filter bar */}
      <div className="flex items-center gap-2 px-4 py-2 border-b border-surface-border bg-surface-panel flex-wrap">
        <MultiSelectPill
          label="Severity"
          options={SEVERITIES}
          selected={filter.severities}
          onChange={(v) => setFilter({ ...filter, severities: v })}
          renderOption={(v) => (
            <span className={clsx('px-1.5 py-0.5 rounded border text-xs', SEV_PILL[v])}>
              {v.charAt(0).toUpperCase() + v.slice(1)}
            </span>
          )}
        />
        <MultiSelectPill
          label="Status"
          options={STATUSES}
          selected={filter.statuses}
          onChange={(v) => setFilter({ ...filter, statuses: v })}
        />
        <MultiSelectPill
          label="Type"
          options={UPDATE_TYPES}
          selected={filter.updateTypes}
          onChange={(v) => setFilter({ ...filter, updateTypes: v })}
          renderOption={(v) => (
            <span className={clsx('px-1 py-0.5 rounded text-xs font-medium', UPDATE_TYPE_STYLES[v])}>
              {updateTypeLabel(v)}
            </span>
          )}
        />

        {/* Cluster filter */}
        <select
          className="input py-1 text-xs h-[28px]"
          value={filter.clusterId}
          onChange={(e) => setFilter({ ...filter, clusterId: e.target.value })}
        >
          <option value="">All clusters</option>
          {clusters.map((c) => (
            <option key={c.id} value={c.id}>{c.display_name}</option>
          ))}
        </select>

        {/* Namespace search */}
        <input
          className="input py-1 text-xs h-[28px] w-40"
          placeholder="Namespace..."
          value={filter.namespace}
          onChange={(e) => setFilter({ ...filter, namespace: e.target.value })}
        />

        <div className="ml-auto flex items-center gap-3">
          <span className="text-xs text-slate-500">
            {total} finding{total !== 1 ? 's' : ''}
          </span>
          <button
            className="btn btn-secondary py-1 px-2 text-xs disabled:opacity-50"
            onClick={() => exportFindingsCsv(apiFilter)}
            disabled={total === 0}
            title="Export filtered findings to CSV"
          >
            <Download size={12} />
            Export
          </button>
        </div>
      </div>

      {/* Bulk action bar */}
      {selectedIds.size > 0 && (
        <div className="flex items-center gap-3 px-4 py-2 bg-blue-950/40 border-b border-blue-900/40">
          <span className="text-xs text-blue-300">{selectedIds.size} selected</span>
          <button
            className="btn btn-secondary py-1 text-xs"
            onClick={() => bulkUpdateStatus('planned')}
          >
            Mark planned
          </button>
          <button
            className="btn btn-secondary py-1 text-xs"
            onClick={() => bulkUpdateStatus('ignored')}
          >
            Mark ignored
          </button>
          <button
            className="ml-auto p-1 text-slate-400 hover:text-slate-200"
            onClick={() => setSelectedIds(new Set())}
          >
            <X size={14} />
          </button>
        </div>
      )}

      {/* No-registry hint: private images aren't scanned without credentials. */}
      {!isLoading && total === 0 && registries.length === 0 && (
        <div className="flex items-center gap-3 px-4 py-3 bg-blue-950/30 border-b border-blue-900/40">
          <Container size={16} className="text-blue-400 flex-shrink-0" />
          <div className="text-xs text-slate-300">
            No update findings yet. Private and self-signed registries aren't scanned until you add credentials.
          </div>
          <Link to="/registries" className="btn btn-secondary py-1 px-2 text-xs ml-auto whitespace-nowrap">
            Configure registries
          </Link>
        </div>
      )}

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <DataTable
          columns={columns}
          data={findings}
          loading={isLoading}
          onRowClick={(f) => setActiveFinding(f)}
          rowKey={(f) => f.id}
          sortBy={filter.sortBy}
          sortDir={filter.sortDir}
          onSort={handleSort}
          selectedIds={selectedIds}
          getRowId={(f) => f.id}
          onSelectRow={handleSelectRow}
          onSelectAll={handleSelectAll}
        />
      </div>

      {/* Pagination */}
      {total > filter.pageSize && (
        <div className="flex items-center justify-between px-4 py-2 border-t border-surface-border bg-surface-panel">
          <span className="text-xs text-slate-500">
            {filter.page * filter.pageSize + 1}–{Math.min((filter.page + 1) * filter.pageSize, total)} of {total}
          </span>
          <div className="flex items-center gap-2">
            <button
              className="btn btn-secondary py-1 px-2 text-xs"
              disabled={filter.page === 0}
              onClick={() => setFilter({ ...filter, page: filter.page - 1 })}
            >
              Previous
            </button>
            <button
              className="btn btn-secondary py-1 px-2 text-xs"
              disabled={(filter.page + 1) * filter.pageSize >= total}
              onClick={() => setFilter({ ...filter, page: filter.page + 1 })}
            >
              Next
            </button>
          </div>
        </div>
      )}

      {/* Finding detail slide-over */}
      <SlideOver
        open={!!activeFinding}
        onClose={() => setActiveFinding(null)}
        title={activeFinding ? (activeFinding.workload_name || activeFinding.target_id) : ''}
        width="42%"
      >
        {activeFinding && <FindingDetail finding={activeFinding} />}
      </SlideOver>
    </div>
  )
}
