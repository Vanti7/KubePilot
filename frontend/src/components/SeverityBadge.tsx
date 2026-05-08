import clsx from 'clsx'
import type { Severity } from '../types'
import { severityToLabel } from '../utils/formatting'

const severityStyles: Record<Severity, string> = {
  critical: 'bg-red-950/80 text-red-400 border border-red-900/60',
  high: 'bg-orange-950/80 text-orange-400 border border-orange-900/60',
  medium: 'bg-yellow-950/80 text-yellow-400 border border-yellow-900/60',
  low: 'bg-blue-950/80 text-blue-400 border border-blue-900/60',
  info: 'bg-slate-800 text-slate-400 border border-slate-700',
}

const dotStyles: Record<Severity, string> = {
  critical: 'bg-severity-critical',
  high: 'bg-severity-high',
  medium: 'bg-severity-medium',
  low: 'bg-severity-low',
  info: 'bg-severity-info',
}

interface Props {
  severity: Severity
  size?: 'sm' | 'md'
  showDot?: boolean
}

export function SeverityBadge({ severity, size = 'sm', showDot = false }: Props) {
  return (
    <span
      className={clsx(
        'inline-flex items-center gap-1 font-medium rounded',
        severityStyles[severity],
        size === 'sm' ? 'px-1.5 py-0.5 text-xs' : 'px-2 py-1 text-sm'
      )}
    >
      {showDot && <span className={clsx('w-1.5 h-1.5 rounded-full flex-shrink-0', dotStyles[severity])} />}
      {severityToLabel(severity)}
    </span>
  )
}
