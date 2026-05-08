import React from 'react'
import { Outlet } from 'react-router-dom'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { User, LogOut, Settings } from 'lucide-react'
import { Sidebar } from './Sidebar'
import { ClusterSelector } from './ClusterSelector'
import { useAuth } from '../contexts/AuthContext'

export function Layout() {
  const { user, logout } = useAuth()

  return (
    <div className="flex h-screen bg-surface-base overflow-hidden">
      {/* Sidebar — fixed left, 220px */}
      <div className="w-[220px] flex-shrink-0 flex flex-col h-full">
        <Sidebar />
      </div>

      {/* Main content */}
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        {/* Top bar */}
        <header className="flex items-center justify-between px-4 py-2 bg-surface-panel border-b border-surface-border flex-shrink-0 h-11">
          <div className="flex items-center gap-3">
            {/* Breadcrumb / page title placeholder — filled by each page */}
            <div id="page-title-portal" className="flex items-center" />
          </div>

          <div className="flex items-center gap-2">
            <ClusterSelector />

            {/* User menu */}
            <DropdownMenu.Root>
              <DropdownMenu.Trigger asChild>
                <button className="flex items-center gap-1.5 px-2 py-1.5 rounded hover:bg-surface-elevated transition-colors text-slate-400 hover:text-slate-200">
                  <User size={15} />
                  <span className="text-xs max-w-[120px] truncate">{user?.email || 'Account'}</span>
                </button>
              </DropdownMenu.Trigger>
              <DropdownMenu.Portal>
                <DropdownMenu.Content
                  className="z-50 min-w-[160px] bg-surface-elevated border border-surface-border rounded-lg shadow-xl py-1 text-sm"
                  sideOffset={4}
                  align="end"
                >
                  <div className="px-3 py-2 border-b border-surface-border">
                    <p className="text-xs text-slate-300 font-medium truncate">{user?.name}</p>
                    <p className="text-xs text-slate-500 truncate">{user?.email}</p>
                  </div>
                  <DropdownMenu.Item
                    className="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-surface-panel outline-none text-slate-400 hover:text-slate-200"
                    onSelect={() => {}}
                  >
                    <Settings size={14} />
                    Settings
                  </DropdownMenu.Item>
                  <DropdownMenu.Separator className="my-1 border-t border-surface-border" />
                  <DropdownMenu.Item
                    className="flex items-center gap-2 px-3 py-2 cursor-pointer hover:bg-surface-panel outline-none text-red-400 hover:text-red-300"
                    onSelect={logout}
                  >
                    <LogOut size={14} />
                    Sign out
                  </DropdownMenu.Item>
                </DropdownMenu.Content>
              </DropdownMenu.Portal>
            </DropdownMenu.Root>
          </div>
        </header>

        {/* Page content */}
        <main className="flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
