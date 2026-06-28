import React, { useEffect, useMemo, useRef, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { useNavigate } from 'react-router-dom'
import {
  Search,
  LayoutDashboard,
  ArrowUpCircle,
  ShieldAlert,
  Boxes,
  Server,
  Package,
  KeyRound,
  Network,
  Container,
  Plug,
  Settings as SettingsIcon,
  CornerDownLeft,
} from 'lucide-react'
import clsx from 'clsx'
import { useClusters } from '../hooks/useClusters'
import { useAuth } from '../contexts/AuthContext'

interface PaletteItem {
  id: string
  label: string
  hint: string
  to: string
  icon: React.ReactNode
}

export function CommandPalette() {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const navigate = useNavigate()
  const { data: clusters = [] } = useClusters()
  const { user } = useAuth()
  const listRef = useRef<HTMLDivElement>(null)

  // Global Cmd/Ctrl+K toggles the palette.
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOpen((o) => !o)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const items: PaletteItem[] = useMemo(() => {
    const nav: PaletteItem[] = [
      { id: 'nav-overview', label: 'Overview', hint: 'Page', to: '/', icon: <LayoutDashboard size={15} /> },
      { id: 'nav-updates', label: 'Updates', hint: 'Page', to: '/updates', icon: <ArrowUpCircle size={15} /> },
      { id: 'nav-risks', label: 'Risks', hint: 'Page', to: '/risks', icon: <ShieldAlert size={15} /> },
      { id: 'nav-inventory', label: 'Workloads', hint: 'Page', to: '/inventory', icon: <Boxes size={15} /> },
      { id: 'nav-nodes', label: 'Nodes', hint: 'Page', to: '/nodes', icon: <Server size={15} /> },
      { id: 'nav-helm', label: 'Helm', hint: 'Page', to: '/helm', icon: <Package size={15} /> },
      { id: 'nav-secrets', label: 'Secrets', hint: 'Page', to: '/secrets', icon: <KeyRound size={15} /> },
      { id: 'nav-clusters', label: 'Clusters', hint: 'Page', to: '/clusters', icon: <Network size={15} /> },
      { id: 'nav-registries', label: 'Registries', hint: 'Page', to: '/registries', icon: <Container size={15} /> },
      { id: 'nav-integrations', label: 'Integrations', hint: 'Page', to: '/integrations', icon: <Plug size={15} /> },
    ]
    if (user?.role === 'admin') {
      nav.push({ id: 'nav-settings', label: 'Settings', hint: 'Page', to: '/settings', icon: <SettingsIcon size={15} /> })
    }
    const clusterItems: PaletteItem[] = clusters.map((c) => ({
      id: `cluster-${c.id}`,
      label: c.display_name || c.name,
      hint: 'Cluster',
      to: `/clusters/${c.id}`,
      icon: <Network size={15} />,
    }))
    return [...nav, ...clusterItems]
  }, [clusters, user])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return items
    return items.filter((i) => i.label.toLowerCase().includes(q) || i.hint.toLowerCase().includes(q))
  }, [items, query])

  // Reset selection/query when the result set changes or the palette opens.
  useEffect(() => setActive(0), [query, open])

  function go(item?: PaletteItem) {
    if (!item) return
    setOpen(false)
    setQuery('')
    navigate(item.to)
  }

  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((a) => Math.min(a + 1, filtered.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((a) => Math.max(a - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      go(filtered[active])
    }
  }

  // Keep the active row visible.
  useEffect(() => {
    const el = listRef.current?.querySelector(`[data-idx="${active}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  }, [active])

  return (
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 bg-black/50 z-50" />
        <Dialog.Content
          className="fixed top-[15%] left-1/2 -translate-x-1/2 z-50 w-full max-w-lg bg-surface-panel border border-surface-border rounded-xl shadow-2xl overflow-hidden"
          onKeyDown={onKeyDown}
          aria-describedby={undefined}
        >
          <Dialog.Title className="sr-only">Command palette</Dialog.Title>
          <div className="flex items-center gap-2 px-3 border-b border-surface-border">
            <Search size={15} className="text-slate-500 flex-shrink-0" />
            <input
              autoFocus
              className="flex-1 bg-transparent py-3 text-sm text-slate-100 placeholder-slate-600 outline-none"
              placeholder="Jump to a page or cluster…"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <kbd className="text-xs text-slate-600 border border-surface-border rounded px-1.5 py-0.5">esc</kbd>
          </div>

          <div ref={listRef} className="max-h-80 overflow-y-auto py-1">
            {filtered.length === 0 ? (
              <div className="px-4 py-6 text-center text-xs text-slate-600">No results</div>
            ) : (
              filtered.map((item, idx) => (
                <button
                  key={item.id}
                  data-idx={idx}
                  className={clsx(
                    'w-full flex items-center gap-2.5 px-3 py-2 text-sm text-left transition-colors',
                    idx === active ? 'bg-surface-elevated text-slate-100' : 'text-slate-400 hover:bg-surface-elevated/50'
                  )}
                  onMouseEnter={() => setActive(idx)}
                  onClick={() => go(item)}
                >
                  <span className="flex-shrink-0 text-slate-500">{item.icon}</span>
                  <span className="flex-1 truncate">{item.label}</span>
                  <span className="text-xs text-slate-600">{item.hint}</span>
                  {idx === active && <CornerDownLeft size={12} className="text-slate-500" />}
                </button>
              ))
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
