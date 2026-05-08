import clsx from 'clsx'
import type { FindingStatus } from '../types'

const statusStyles: Record<FindingStatus, string> = {
  open: 'bg-blue-950/80 text-blue-400 border border-blue-900/60',
  planned: 'bg-purple-950/80 text-purple-400 border border-purple-900/60',
  ignored: 'bg-slate-800 text-slate-500 border border-slate-700',
  approved: 'bg-green-950/80 text-green-400 border border-green-900/60',
  blocked: 'bg-red-950/80 text-red-400 border border-red-900/60',
  resolved: 'bg-teal-950/80 text-teal-400 border border-teal-900/60',
}

const statusLabels: Record<FindingStatus, string> = {
  open: 'Open',
  planned: 'Planned',
  ignored: 'Ignored',
  approved: 'Approved',
  blocked: 'Blocked',
  resolved: 'Resolved',
}

interface Props {
  status: FindingStatus
  size?: 'sm' | 'md'
}

export function StatusBadge({ status, size = 'sm' }: Props) {
  return (
    <span
      className={clsx(
        'inline-flex items-center font-medium rounded',
        statusStyles[status],
        size === 'sm' ? 'px-1.5 py-0.5 text-xs' : 'px-2 py-1 text-sm'
      )}
    >
      {statusLabels[status]}
    </span>
  )
}
