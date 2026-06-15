import { Wifi, WifiOff, AlertTriangle, HelpCircle } from 'lucide-react'
import clsx from 'clsx'
import type { ClusterStatus } from '../types'

const STATUS_COLOR: Record<ClusterStatus, string> = {
  healthy: 'text-green-400',
  unreachable: 'text-red-400',
  degraded: 'text-yellow-400',
  unknown: 'text-slate-500',
}

// ClusterStatusIcon renders the connectivity icon for a cluster status.
export function ClusterStatusIcon({ status, size = 13 }: { status: ClusterStatus; size?: number }) {
  const cls = STATUS_COLOR[status] ?? STATUS_COLOR.unknown
  switch (status) {
    case 'healthy':
      return <Wifi size={size} className={cls} />
    case 'unreachable':
      return <WifiOff size={size} className={cls} />
    case 'degraded':
      return <AlertTriangle size={size} className={cls} />
    default:
      return <HelpCircle size={size} className={cls} />
  }
}

// ClusterStatusBadge renders an icon + colored, capitalized status label.
export function ClusterStatusBadge({ status, size = 13 }: { status: ClusterStatus; size?: number }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <ClusterStatusIcon status={status} size={size} />
      <span className={clsx('text-xs font-medium capitalize', STATUS_COLOR[status] ?? STATUS_COLOR.unknown)}>
        {status}
      </span>
    </span>
  )
}
