// Shared resource-usage primitives used by the dashboard and cluster detail.

// bytesToHuman renders a byte count as a human-readable MiB/GiB/TiB string.
export function bytesToHuman(n: number): string {
  const gib = n / 1024 ** 3
  if (gib >= 1024) return `${(gib / 1024).toFixed(1)} TiB`
  if (gib >= 1) return `${gib.toFixed(1)} GiB`
  return `${(n / 1024 ** 2).toFixed(0)} MiB`
}

// usageColor maps a usage percentage to a threshold color (green/amber/red).
export function usageColor(pct: number): string {
  return pct >= 85 ? '#f87171' : pct >= 60 ? '#fbbf24' : '#34d399'
}

// ResourceGauge is a Proxmox/vCenter-style radial gauge for a usage percentage.
export function ResourceGauge({ label, percent, detail }: { label: string; percent: number; detail: string }) {
  const r = 34
  const circ = 2 * Math.PI * r
  const pct = Math.min(Math.max(percent, 0), 100)
  const offset = circ * (1 - pct / 100)
  return (
    <div className="flex flex-col items-center gap-1">
      <div className="relative w-24 h-24">
        <svg viewBox="0 0 80 80" className="w-24 h-24 -rotate-90">
          <circle cx="40" cy="40" r={r} fill="none" stroke="#1e293b" strokeWidth="8" />
          <circle
            cx="40"
            cy="40"
            r={r}
            fill="none"
            stroke={usageColor(pct)}
            strokeWidth="8"
            strokeDasharray={circ}
            strokeDashoffset={offset}
            strokeLinecap="round"
          />
        </svg>
        <div className="absolute inset-0 flex items-center justify-center">
          <span className="text-lg font-bold font-mono text-slate-100">{pct.toFixed(0)}%</span>
        </div>
      </div>
      <div className="text-xs font-medium text-slate-300">{label}</div>
      <div className="text-xs text-slate-500 font-mono">{detail}</div>
    </div>
  )
}
