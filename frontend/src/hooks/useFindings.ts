import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getFindings, getFindingSummary } from '../api/client'
import type { FindingFilter, Severity, FindingStatus, UpdateType } from '../types'

export function useFindings(filter: FindingFilter = {}) {
  return useQuery({
    queryKey: ['findings', filter],
    queryFn: () => getFindings(filter),
  })
}

export function useFindingSummary() {
  return useQuery({
    queryKey: ['findings', 'summary'],
    queryFn: getFindingSummary,
  })
}

interface FindingsFilterState {
  severities: Severity[]
  statuses: FindingStatus[]
  updateTypes: UpdateType[]
  clusterId: string
  namespace: string
  page: number
  pageSize: number
  sortBy: string
  sortDir: 'asc' | 'desc'
}

export function useFindingsFilter() {
  const [filter, setFilter] = useState<FindingsFilterState>({
    severities: [],
    statuses: ['open'],
    updateTypes: [],
    clusterId: '',
    namespace: '',
    page: 0,
    pageSize: 50,
    sortBy: 'score',
    sortDir: 'desc',
  })

  const apiFilter: FindingFilter = {
    severity: filter.severities.length > 0 ? filter.severities : undefined,
    status: filter.statuses.length > 0 ? filter.statuses : undefined,
    update_type: filter.updateTypes.length > 0 ? filter.updateTypes : undefined,
    cluster_id: filter.clusterId || undefined,
    namespace: filter.namespace || undefined,
    limit: filter.pageSize,
    offset: filter.page * filter.pageSize,
    sort_by: filter.sortBy,
    sort_dir: filter.sortDir,
  }

  return { filter, setFilter, apiFilter }
}
