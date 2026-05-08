import { useQuery } from '@tanstack/react-query'
import { getClusters, getCluster } from '../api/client'

export function useClusters() {
  return useQuery({
    queryKey: ['clusters'],
    queryFn: getClusters,
  })
}

export function useCluster(id: string) {
  return useQuery({
    queryKey: ['cluster', id],
    queryFn: () => getCluster(id),
    enabled: !!id,
  })
}
