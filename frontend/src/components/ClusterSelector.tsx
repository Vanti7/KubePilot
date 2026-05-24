import React from 'react'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { ChevronDown, Wifi, WifiOff, AlertTriangle } from 'lucide-react'
import clsx from 'clsx'
import { useClusters } from '../hooks/useClusters'
import { useClusterContext } from '../contexts/ClusterContext'
import type { ClusterStatus } from '../types'

const statusDot: Record<ClusterStatus, { icon: React.ReactNode; color: string }> = {
  healthy: { icon: <Wifi size={12} />, color: 'text-green-400' },
  unreachable: { icon: <WifiOff size={12} />, color: 'text-red-400' },
  degraded: { icon: <AlertTriangle size={12} />, color: 'text-yellow-400' },
}

export function ClusterSelector() {
  const { data: clusters = [] } = useClusters()
  const { selectedClusterIds, toggleCluster, clearSelection } = useClusterContext()

  const label =
    selectedClusterIds.length === 0
      ? 'All clusters'
      : selectedClusterIds.length === 1
      ? clusters.find((c) => c.id === selectedClusterIds[0])?.display_name || '1 cluster'
      : `${selectedClusterIds.length} clusters`

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <button className="inline-flex items-center gap-2 px-3 py-1.5 rounded bg-surface-elevated border border-surface-border text-sm text-slate-300 hover:text-slate-100 hover:border-surface-border/80 transition-colors">
          <span className="max-w-[160px] truncate">{label}</span>
          <ChevronDown size={14} className="text-slate-500 flex-shrink-0" />
        </button>
      </DropdownMenu.Trigger>

      <DropdownMenu.Portal>
        <DropdownMenu.Content
          className="z-50 min-w-[220px] bg-surface-elevated border border-surface-border rounded-lg shadow-xl py-1 text-sm"
          sideOffset={4}
          align="end"
        >
          <DropdownMenu.Item
            className="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-surface-panel outline-none text-slate-300 hover:text-slate-100"
            onSelect={(e) => {
              e.preventDefault()
              clearSelection()
            }}
          >
            <span className="w-4 flex-shrink-0">
              {selectedClusterIds.length === 0 && (
                <span className="w-3 h-3 rounded-full bg-blue-500 block" />
              )}
            </span>
            <span>All clusters</span>
            <span className="ml-auto text-xs text-slate-500">{clusters.length}</span>
          </DropdownMenu.Item>

          {clusters.length > 0 && (
            <DropdownMenu.Separator className="my-1 border-t border-surface-border" />
          )}

          {clusters.map((cluster) => {
            const isSelected = selectedClusterIds.includes(cluster.id)
            const dot = statusDot[cluster.status]
            return (
              <DropdownMenu.Item
                key={cluster.id}
                className="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-surface-panel outline-none text-slate-300 hover:text-slate-100"
                onSelect={(e) => {
                  e.preventDefault()
                  toggleCluster(cluster.id)
                }}
              >
                <span className="w-4 flex-shrink-0">
                  <input
                    type="checkbox"
                    className="rounded border-surface-border bg-surface-base accent-blue-500 pointer-events-none"
                    checked={isSelected}
                    readOnly
                  />
                </span>
                <span className="flex-1 truncate">{cluster.display_name}</span>
                <span className={clsx('flex-shrink-0', dot.color)}>{dot.icon}</span>
              </DropdownMenu.Item>
            )
          })}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
