import { formatDistanceToNow, differenceInDays, differenceInHours, differenceInMinutes } from 'date-fns'
import type { Severity } from '../types'

export function formatAge(date: string): string {
  const d = new Date(date)
  const now = new Date()
  const mins = differenceInMinutes(now, d)
  if (mins < 60) return `${mins}m`
  const hours = differenceInHours(now, d)
  if (hours < 24) return `${hours}h`
  const days = differenceInDays(now, d)
  if (days < 60) return `${days}d`
  const months = Math.floor(days / 30)
  return `${months}mo`
}

export function formatRelative(date: string): string {
  try {
    return formatDistanceToNow(new Date(date), { addSuffix: true })
  } catch {
    return '—'
  }
}

export function formatVersion(current: string, available: string): string {
  return `${current} → ${available}`
}

export function scoreToColor(score: number): string {
  if (score >= 80) return 'text-severity-critical'
  if (score >= 60) return 'text-severity-high'
  if (score >= 40) return 'text-severity-medium'
  if (score >= 20) return 'text-severity-low'
  return 'text-severity-info'
}

export function scoreToBg(score: number): string {
  if (score >= 80) return 'bg-severity-critical'
  if (score >= 60) return 'bg-severity-high'
  if (score >= 40) return 'bg-severity-medium'
  if (score >= 20) return 'bg-severity-low'
  return 'bg-severity-info'
}

export function severityToLabel(s: Severity): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

export function severityOrder(s: Severity): number {
  const order: Record<Severity, number> = { critical: 0, high: 1, medium: 2, low: 3, info: 4 }
  return order[s] ?? 99
}

export function buildHeadlampURL(
  baseURL: string,
  cluster: string,
  namespace: string,
  kind: string,
  name: string
): string {
  const kindPath = kind.toLowerCase() + 's'
  if (namespace) {
    return `${baseURL}/c/${cluster}/${kindPath}/${namespace}/${name}`
  }
  return `${baseURL}/c/${cluster}/${kindPath}/${name}`
}

export function formatBytes(bytes: string): string {
  const num = parseInt(bytes, 10)
  if (isNaN(num)) return bytes
  if (num >= 1024 * 1024 * 1024) return `${(num / (1024 * 1024 * 1024)).toFixed(1)}Gi`
  if (num >= 1024 * 1024) return `${(num / (1024 * 1024)).toFixed(0)}Mi`
  if (num >= 1024) return `${(num / 1024).toFixed(0)}Ki`
  return `${num}`
}

export function updateTypeLabel(t: string): string {
  return t.toUpperCase()
}

export function clusterStatusLabel(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}
