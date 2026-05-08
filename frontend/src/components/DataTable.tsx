import React from 'react'
import clsx from 'clsx'
import { ChevronUp, ChevronDown } from 'lucide-react'

export interface Column<T> {
  key: string
  header: string
  render: (row: T, index: number) => React.ReactNode
  sortable?: boolean
  width?: string
  align?: 'left' | 'right' | 'center'
}

interface Props<T> {
  columns: Column<T>[]
  data: T[]
  loading?: boolean
  onRowClick?: (row: T) => void
  rowKey?: (row: T, index: number) => string
  sortBy?: string
  sortDir?: 'asc' | 'desc'
  onSort?: (key: string) => void
  selectedIds?: Set<string>
  getRowId?: (row: T) => string
  onSelectRow?: (id: string, checked: boolean) => void
  onSelectAll?: (checked: boolean) => void
  emptyMessage?: string
  className?: string
}

const SKELETON_ROWS = 8
const SKELETON_WIDTHS = [70, 55, 80, 65, 75, 60, 85, 50]

export function DataTable<T>({
  columns,
  data,
  loading,
  onRowClick,
  rowKey,
  sortBy,
  sortDir,
  onSort,
  selectedIds,
  getRowId,
  onSelectRow,
  onSelectAll,
  emptyMessage = 'No data',
  className,
}: Props<T>) {
  const showSelection = !!(selectedIds && getRowId && onSelectRow)
  const allSelected = showSelection && data.length > 0 && data.every((r) => selectedIds.has(getRowId!(r)))
  const someSelected = showSelection && data.some((r) => selectedIds.has(getRowId!(r)))

  return (
    <div className={clsx('overflow-x-auto', className)}>
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-surface-border">
            {showSelection && (
              <th className="w-8 px-3 py-2 text-left">
                <input
                  type="checkbox"
                  className="rounded border-surface-border bg-surface-elevated accent-blue-500"
                  checked={allSelected}
                  ref={(el) => { if (el) el.indeterminate = someSelected && !allSelected }}
                  onChange={(e) => onSelectAll?.(e.target.checked)}
                />
              </th>
            )}
            {columns.map((col) => (
              <th
                key={col.key}
                className={clsx(
                  'px-3 py-2 text-xs font-medium text-slate-400 uppercase tracking-wider whitespace-nowrap',
                  col.align === 'right' ? 'text-right' : col.align === 'center' ? 'text-center' : 'text-left',
                  col.sortable && 'cursor-pointer hover:text-slate-200 select-none',
                  col.width
                )}
                style={col.width ? { width: col.width } : undefined}
                onClick={() => col.sortable && onSort?.(col.key)}
              >
                <span className="inline-flex items-center gap-1">
                  {col.header}
                  {col.sortable && sortBy === col.key && (
                    sortDir === 'asc' ? <ChevronUp size={12} /> : <ChevronDown size={12} />
                  )}
                  {col.sortable && sortBy !== col.key && (
                    <span className="opacity-0 group-hover:opacity-50">
                      <ChevronDown size={12} />
                    </span>
                  )}
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {loading ? (
            Array.from({ length: SKELETON_ROWS }).map((_, i) => (
              <tr key={i} className="border-b border-surface-border/50">
                {showSelection && (
                  <td className="px-3 py-2.5">
                    <div className="skeleton w-4 h-4 rounded" />
                  </td>
                )}
                {columns.map((col, ci) => (
                  <td key={col.key} className="px-3 py-2.5">
                    <div className="skeleton h-4 rounded" style={{ width: `${SKELETON_WIDTHS[(i + ci) % SKELETON_WIDTHS.length]}%` }} />
                  </td>
                ))}
              </tr>
            ))
          ) : data.length === 0 ? (
            <tr>
              <td
                colSpan={columns.length + (showSelection ? 1 : 0)}
                className="px-3 py-8 text-center text-slate-500 text-sm"
              >
                {emptyMessage}
              </td>
            </tr>
          ) : (
            data.map((row, i) => {
              const id = getRowId ? getRowId(row) : undefined
              const isSelected = id ? selectedIds?.has(id) : false
              return (
                <tr
                  key={rowKey ? rowKey(row, i) : i}
                  className={clsx(
                    'border-b border-surface-border/50 transition-colors duration-75',
                    onRowClick && 'cursor-pointer hover:bg-surface-elevated',
                    isSelected && 'bg-blue-950/20'
                  )}
                  onClick={() => onRowClick?.(row)}
                >
                  {showSelection && id && (
                    <td
                      className="px-3 py-2.5"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <input
                        type="checkbox"
                        className="rounded border-surface-border bg-surface-elevated accent-blue-500"
                        checked={isSelected}
                        onChange={(e) => onSelectRow(id, e.target.checked)}
                      />
                    </td>
                  )}
                  {columns.map((col) => (
                    <td
                      key={col.key}
                      className={clsx(
                        'px-3 py-2.5',
                        col.align === 'right' ? 'text-right' : col.align === 'center' ? 'text-center' : ''
                      )}
                    >
                      {col.render(row, i)}
                    </td>
                  ))}
                </tr>
              )
            })
          )}
        </tbody>
      </table>
    </div>
  )
}
