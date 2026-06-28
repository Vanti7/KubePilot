import React, { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as Dialog from '@radix-ui/react-dialog'
import { Plus, Trash2, X, ShieldCheck, Users as UsersIcon, SlidersHorizontal } from 'lucide-react'
import clsx from 'clsx'
import { getUsers, createUser, updateUser, deleteUser, getSettings } from '../api/client'
import { useAuth } from '../contexts/AuthContext'
import type { User } from '../types'
import { formatRelative } from '../utils/formatting'

const ROLES = ['admin', 'operator', 'viewer']

const ROLE_STYLES: Record<string, string> = {
  admin: 'bg-purple-950/80 text-purple-300 border-purple-900/60',
  operator: 'bg-blue-950/80 text-blue-300 border-blue-900/60',
  viewer: 'bg-slate-800 text-slate-400 border-slate-700',
}

function AddUserModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient()
  const [email, setEmail] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState('viewer')
  const [error, setError] = useState('')

  const { mutate, isPending } = useMutation({
    mutationFn: createUser,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] })
      onClose()
    },
    onError: (e: any) => setError(e?.response?.data?.error || e.message),
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    mutate({ email, name, password, role })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">Name</label>
          <input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} required />
        </div>
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-400">Role</label>
          <select className="input w-full capitalize" value={role} onChange={(e) => setRole(e.target.value)}>
            {ROLES.map((r) => <option key={r} value={r}>{r}</option>)}
          </select>
        </div>
      </div>
      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Email</label>
        <input className="input w-full" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      </div>
      <div className="space-y-1.5">
        <label className="block text-xs font-medium text-slate-400">Password <span className="text-slate-600">(min 8 chars)</span></label>
        <input className="input w-full" type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required autoComplete="new-password" />
      </div>

      {error && <div className="px-3 py-2 rounded bg-red-950/60 border border-red-900/50 text-red-400 text-xs">{error}</div>}

      <div className="flex justify-end gap-2 pt-2">
        <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
        <button type="submit" className="btn btn-primary" disabled={isPending}>{isPending ? 'Adding...' : 'Add user'}</button>
      </div>
    </form>
  )
}

function UsersTab() {
  const qc = useQueryClient()
  const { user: me } = useAuth()
  const { data: users = [], isLoading } = useQuery({ queryKey: ['users'], queryFn: getUsers })
  const [addOpen, setAddOpen] = useState(false)
  const [error, setError] = useState('')

  const { mutate: patch } = useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: { role?: string; is_active?: boolean } }) => updateUser(id, payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
    onError: (e: any) => setError(e?.response?.data?.error || e.message),
  })

  const { mutate: remove } = useMutation({
    mutationFn: deleteUser,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
    onError: (e: any) => setError(e?.response?.data?.error || e.message),
  })

  function handleDelete(u: User) {
    if (!confirm(`Delete user ${u.email}?`)) return
    setError('')
    remove(u.id)
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <span className="text-xs text-slate-500">{users.length} user{users.length !== 1 ? 's' : ''}</span>
        <Dialog.Root open={addOpen} onOpenChange={setAddOpen}>
          <Dialog.Trigger asChild>
            <button className="btn btn-primary py-1 px-2 text-xs"><Plus size={13} /> Add user</button>
          </Dialog.Trigger>
          <Dialog.Portal>
            <Dialog.Overlay className="fixed inset-0 bg-black/50 z-40" />
            <Dialog.Content className="fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-50 w-full max-w-md bg-surface-panel border border-surface-border rounded-xl shadow-2xl p-6">
              <div className="flex items-center justify-between mb-4">
                <Dialog.Title className="text-sm font-semibold text-slate-100">Add user</Dialog.Title>
                <Dialog.Close asChild>
                  <button className="p-1 rounded hover:bg-surface-elevated text-slate-400 hover:text-slate-200"><X size={15} /></button>
                </Dialog.Close>
              </div>
              {addOpen && <AddUserModal onClose={() => setAddOpen(false)} />}
            </Dialog.Content>
          </Dialog.Portal>
        </Dialog.Root>
      </div>

      {error && <div className="px-3 py-2 rounded bg-red-950/60 border border-red-900/50 text-red-400 text-xs">{error}</div>}

      <div className="panel overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-xs text-slate-500 border-b border-surface-border">
              <th className="px-4 py-2.5 font-medium">User</th>
              <th className="px-4 py-2.5 font-medium">Role</th>
              <th className="px-4 py-2.5 font-medium">Status</th>
              <th className="px-4 py-2.5 font-medium">Last login</th>
              <th className="px-4 py-2.5 font-medium text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {isLoading ? (
              <tr><td colSpan={5} className="px-4 py-6 text-center text-slate-600 text-xs">Loading…</td></tr>
            ) : users.map((u) => {
              const isSelf = u.id === me?.id
              return (
                <tr key={u.id} className="border-b border-surface-border/60 last:border-0 hover:bg-surface-elevated/40">
                  <td className="px-4 py-2.5">
                    <div className="text-slate-200 font-medium">{u.name}{isSelf && <span className="ml-2 text-xs text-slate-500">(you)</span>}</div>
                    <div className="text-xs text-slate-500">{u.email}</div>
                  </td>
                  <td className="px-4 py-2.5">
                    <select
                      className={clsx('px-1.5 py-0.5 rounded text-xs border capitalize bg-transparent', ROLE_STYLES[u.role])}
                      value={u.role}
                      onChange={(e) => patch({ id: u.id, payload: { role: e.target.value } })}
                    >
                      {ROLES.map((r) => <option key={r} value={r} className="bg-surface-panel text-slate-200">{r}</option>)}
                    </select>
                  </td>
                  <td className="px-4 py-2.5">
                    <button
                      className={clsx('inline-flex items-center gap-1.5 text-xs', u.is_active !== false ? 'text-green-400' : 'text-slate-500')}
                      onClick={() => patch({ id: u.id, payload: { is_active: u.is_active === false } })}
                      disabled={isSelf}
                      title={isSelf ? 'You cannot deactivate yourself' : 'Toggle active'}
                    >
                      <span className={clsx('w-2 h-2 rounded-full', u.is_active !== false ? 'bg-green-400' : 'bg-slate-600')} />
                      {u.is_active !== false ? 'Active' : 'Disabled'}
                    </button>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-slate-500">{u.last_login_at ? formatRelative(u.last_login_at) : 'never'}</td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center justify-end">
                      <button
                        className="btn btn-danger py-1 px-2 text-xs disabled:opacity-40"
                        onClick={() => handleDelete(u)}
                        disabled={isSelf}
                        title={isSelf ? 'You cannot delete yourself' : 'Delete user'}
                      >
                        <Trash2 size={12} />
                      </button>
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function SettingRow({ label, value, mono }: { label: string; value: React.ReactNode; mono?: boolean }) {
  return (
    <div className="flex items-center justify-between px-4 py-2.5 border-b border-surface-border/60 last:border-0">
      <span className="text-xs text-slate-500">{label}</span>
      <span className={clsx('text-xs text-slate-300', mono && 'font-mono')}>{value}</span>
    </div>
  )
}

function BoolPill({ on }: { on: boolean }) {
  return (
    <span className={clsx('px-1.5 py-0.5 rounded text-xs border', on ? 'bg-green-950/60 text-green-400 border-green-900/50' : 'bg-slate-800 text-slate-500 border-slate-700')}>
      {on ? 'enabled' : 'disabled'}
    </span>
  )
}

function SystemTab() {
  const { data, isLoading } = useQuery({ queryKey: ['settings'], queryFn: getSettings })
  if (isLoading || !data) return <div className="panel p-4"><div className="skeleton h-32 w-full rounded" /></div>

  return (
    <div className="space-y-4">
      <div className="panel">
        <div className="px-4 py-2.5 border-b border-surface-border">
          <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">Runtime</h2>
        </div>
        <SettingRow label="Version" value={data.version} mono />
        <SettingRow label="Storage driver" value={data.storage_driver} mono />
        <SettingRow label="Cache driver" value={data.cache_driver} mono />
        <SettingRow label="Local mode" value={<BoolPill on={data.local_mode} />} />
        <SettingRow label="Demo mode" value={<BoolPill on={data.demo_mode} />} />
        <SettingRow label="In-cluster" value={<BoolPill on={data.in_cluster} />} />
        <SettingRow label="MCP writes allowed" value={<BoolPill on={data.mcp_allow_writes} />} />
      </div>

      <div className="panel">
        <div className="px-4 py-2.5 border-b border-surface-border">
          <h2 className="text-xs font-semibold text-slate-300 uppercase tracking-wider">Collection</h2>
        </div>
        <SettingRow label="Worker interval" value={`${data.worker_interval_seconds}s`} mono />
        <SettingRow label="Node metrics" value={<BoolPill on={data.node_metrics_enabled} />} />
        <SettingRow label="Metrics retention" value={`${data.node_metrics_retention_hours}h`} mono />
      </div>

      <p className="text-xs text-slate-600 flex items-center gap-1.5">
        <ShieldCheck size={13} /> These values are read-only and set via environment variables. Secrets are never shown.
      </p>
    </div>
  )
}

type Tab = 'users' | 'system'

export function Settings() {
  const { user } = useAuth()
  const [tab, setTab] = useState<Tab>('users')

  if (user && user.role !== 'admin') {
    return (
      <div className="p-4 max-w-3xl mx-auto">
        <div className="panel p-12 flex flex-col items-center gap-3">
          <ShieldCheck size={28} className="text-slate-600" />
          <p className="text-sm text-slate-400">Settings are restricted to administrators.</p>
        </div>
      </div>
    )
  }

  const tabs: { id: Tab; label: string; icon: React.ReactNode }[] = [
    { id: 'users', label: 'Users', icon: <UsersIcon size={14} /> },
    { id: 'system', label: 'System', icon: <SlidersHorizontal size={14} /> },
  ]

  return (
    <div className="p-4 max-w-4xl mx-auto space-y-4">
      <h1 className="text-base font-semibold text-slate-100">Settings</h1>

      <div className="flex items-center gap-1 border-b border-surface-border">
        {tabs.map((t) => (
          <button
            key={t.id}
            className={clsx(
              'flex items-center gap-1.5 px-3 py-2 text-sm border-b-2 -mb-px transition-colors',
              tab === t.id ? 'border-blue-500 text-slate-100' : 'border-transparent text-slate-400 hover:text-slate-200'
            )}
            onClick={() => setTab(t.id)}
          >
            {t.icon}
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'users' ? <UsersTab /> : <SystemTab />}
    </div>
  )
}
