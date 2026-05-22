import { useState } from 'react'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Check } from 'lucide-react'
import { updateFindingStatus } from '../api/client'
import { StatusBadge } from './StatusBadge'
import type { FindingStatus } from '../types'

const STATUS_OPTIONS: { value: FindingStatus; label: string; description: string }[] = [
  { value: 'open', label: 'Open', description: 'Needs attention' },
  { value: 'planned', label: 'Planned', description: 'Scheduled for update' },
  { value: 'approved', label: 'Approved', description: 'Update approved' },
  { value: 'blocked', label: 'Blocked', description: 'Cannot update now' },
  { value: 'ignored', label: 'Ignored', description: 'Acceptable risk' },
  { value: 'resolved', label: 'Resolved', description: 'Update applied' },
]

interface Props {
  findingId: string
  currentStatus: FindingStatus
  onChanged?: (newStatus: FindingStatus) => void
}

export function FindingStatusMenu({ findingId, currentStatus, onChanged }: Props) {
  const qc = useQueryClient()
  const [loading, setLoading] = useState(false)

  async function handleSelect(status: FindingStatus) {
    if (status === currentStatus) return
    setLoading(true)
    try {
      await updateFindingStatus(findingId, status)
      await qc.invalidateQueries({ queryKey: ['findings'] })
      await qc.invalidateQueries({ queryKey: ['overview'] })
      onChanged?.(status)
    } catch (e) {
      console.error('Failed to update status', e)
    } finally {
      setLoading(false)
    }
  }

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild disabled={loading}>
        <button className="inline-flex items-center gap-1 hover:opacity-80 transition-opacity disabled:opacity-50">
          <StatusBadge status={currentStatus} />
          <ChevronDown size={12} className="text-slate-500" />
        </button>
      </DropdownMenu.Trigger>

      <DropdownMenu.Portal>
        <DropdownMenu.Content
          className="z-50 min-w-[180px] bg-surface-elevated border border-surface-border rounded-lg shadow-xl py-1 text-sm"
          sideOffset={4}
          align="start"
        >
          {STATUS_OPTIONS.map((opt) => (
            <DropdownMenu.Item
              key={opt.value}
              className="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-surface-panel outline-none text-slate-300 hover:text-slate-100"
              onSelect={() => handleSelect(opt.value)}
            >
              <span className="w-4 flex-shrink-0">
                {currentStatus === opt.value && <Check size={12} className="text-blue-400" />}
              </span>
              <span className="flex-1">
                <span className="block font-medium">{opt.label}</span>
                <span className="block text-xs text-slate-500">{opt.description}</span>
              </span>
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
