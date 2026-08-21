import React from 'react'
import { NavLink } from 'react-router-dom'
import {
  LayoutDashboard,
  ArrowUpCircle,
  Boxes,
  Server,
  Package,
  KeyRound,
  ShieldAlert,
  ShieldOff,
  Calendar,
  Clock,
  Plug,
  Network,
  Container,
  Rocket,
  Settings as SettingsIcon,
} from 'lucide-react'
import clsx from 'clsx'
import { useFindingSummary } from '../hooks/useFindings'
import { useAuth } from '../contexts/AuthContext'

interface NavItem {
  to: string
  icon: React.ReactNode
  label: string
  badge?: number
  disabled?: boolean
  tag?: string
}

function CriticalBadge({ count }: { count: number }) {
  if (!count) return null
  return (
    <span className="ml-auto px-1.5 py-0.5 text-xs font-semibold rounded-full bg-red-900/60 text-red-300 border border-red-900/80">
      {count > 99 ? '99+' : count}
    </span>
  )
}

export function Sidebar() {
  const { data: summary } = useFindingSummary()
  const { user } = useAuth()

  const criticalHighCount = summary
    ? (summary.critical || 0) + (summary.high || 0)
    : 0

  const navItems: NavItem[] = [
    { to: '/', icon: <LayoutDashboard size={16} />, label: 'Overview' },
    {
      to: '/updates',
      icon: <ArrowUpCircle size={16} />,
      label: 'Updates',
      badge: criticalHighCount,
    },
  ]

  const canWrite = user?.role === 'admin' || user?.role === 'operator'

  const inventoryItems: NavItem[] = [
    { to: '/inventory', icon: <Boxes size={16} />, label: 'Workloads' },
    { to: '/nodes', icon: <Server size={16} />, label: 'Nodes' },
    { to: '/helm', icon: <Package size={16} />, label: 'Helm' },
    { to: '/secrets', icon: <KeyRound size={16} />, label: 'Secrets' },
    ...(canWrite ? [{ to: '/deploy', icon: <Rocket size={16} />, label: 'Deploy' }] : []),
  ]

  const systemItems: NavItem[] = [
    { to: '/clusters', icon: <Network size={16} />, label: 'Clusters' },
    { to: '/registries', icon: <Container size={16} />, label: 'Registries' },
    { to: '/risks', icon: <ShieldAlert size={16} />, label: 'Risks' },
    { to: '/exception-rules', icon: <ShieldOff size={16} />, label: 'Exceptions' },
    {
      to: '/maintenance',
      icon: <Calendar size={16} />,
      label: 'Maintenance',
      disabled: true,
      tag: 'V2',
    },
    { to: '/history', icon: <Clock size={16} />, label: 'History' },
    { to: '/integrations', icon: <Plug size={16} />, label: 'Integrations' },
    ...(user?.role === 'admin'
      ? [{ to: '/settings', icon: <SettingsIcon size={16} />, label: 'Settings' }]
      : []),
  ]

  return (
    <nav className="flex flex-col h-full bg-surface-panel border-r border-surface-border">
      {/* Logo */}
      <div className="flex items-center gap-2 px-4 py-3.5 border-b border-surface-border">
        <div className="w-6 h-6 rounded bg-blue-600 flex items-center justify-center flex-shrink-0">
          <span className="text-white text-xs font-bold">K</span>
        </div>
        <span className="text-sm font-semibold text-slate-100 tracking-tight">KubePilot</span>
      </div>

      {/* Nav */}
      <div className="flex-1 overflow-y-auto py-2 space-y-0.5 px-2">
        {navItems.map((item) => (
          <SidebarItem key={item.to} item={item} />
        ))}

        <div className="pt-3 pb-1 px-1">
          <span className="text-xs font-medium text-slate-600 uppercase tracking-wider">Inventory</span>
        </div>
        {inventoryItems.map((item) => (
          <SidebarItem key={item.to} item={item} />
        ))}

        <div className="pt-3 pb-1 px-1">
          <span className="text-xs font-medium text-slate-600 uppercase tracking-wider">System</span>
        </div>
        {systemItems.map((item) => (
          <SidebarItem key={item.to} item={item} />
        ))}
      </div>
    </nav>
  )
}

function SidebarItem({ item }: { item: NavItem }) {
  if (item.disabled) {
    return (
      <div className="flex items-center gap-2.5 px-3 py-2 rounded text-sm text-slate-600 cursor-not-allowed select-none">
        <span className="flex-shrink-0">{item.icon}</span>
        <span className="flex-1">{item.label}</span>
        {item.tag && (
          <span className="text-xs px-1 py-0.5 rounded bg-slate-800 text-slate-600 border border-slate-700">
            {item.tag}
          </span>
        )}
      </div>
    )
  }

  return (
    <NavLink
      to={item.to}
      end={item.to === '/'}
      className={({ isActive }) =>
        clsx(
          'flex items-center gap-2.5 px-3 py-2 rounded text-sm transition-colors duration-75',
          isActive
            ? 'bg-surface-elevated text-slate-100 border-l-2 border-blue-500 pl-[10px]'
            : 'text-slate-400 hover:text-slate-200 hover:bg-surface-elevated/60'
        )
      }
    >
      <span className="flex-shrink-0">{item.icon}</span>
      <span className="flex-1">{item.label}</span>
      {item.badge !== undefined && <CriticalBadge count={item.badge} />}
    </NavLink>
  )
}
