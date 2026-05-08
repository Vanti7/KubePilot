import React, { createContext, useContext, useState, useCallback } from 'react'

interface ClusterContextValue {
  selectedClusterIds: string[]
  setSelectedClusterIds: (ids: string[]) => void
  toggleCluster: (id: string) => void
  clearSelection: () => void
}

const ClusterContext = createContext<ClusterContextValue | null>(null)

const STORAGE_KEY = 'kubepilot_selected_clusters'

function loadFromStorage(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    return JSON.parse(raw) as string[]
  } catch {
    return []
  }
}

export function ClusterProvider({ children }: { children: React.ReactNode }) {
  const [selectedClusterIds, setSelectedClusterIdsState] = useState<string[]>(loadFromStorage)

  const setSelectedClusterIds = useCallback((ids: string[]) => {
    setSelectedClusterIdsState(ids)
    localStorage.setItem(STORAGE_KEY, JSON.stringify(ids))
  }, [])

  const toggleCluster = useCallback((id: string) => {
    setSelectedClusterIdsState((prev) => {
      const next = prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
      localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
      return next
    })
  }, [])

  const clearSelection = useCallback(() => {
    setSelectedClusterIdsState([])
    localStorage.removeItem(STORAGE_KEY)
  }, [])

  return (
    <ClusterContext.Provider value={{ selectedClusterIds, setSelectedClusterIds, toggleCluster, clearSelection }}>
      {children}
    </ClusterContext.Provider>
  )
}

export function useClusterContext() {
  const ctx = useContext(ClusterContext)
  if (!ctx) throw new Error('useClusterContext must be used within ClusterProvider')
  return ctx
}
