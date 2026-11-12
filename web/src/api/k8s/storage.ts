import type { AxiosInstance } from 'axios'
import type { Envelope } from '@/types/api'

export type StorageKind = 'persistentvolumes' | 'persistentvolumeclaims' | 'storageclasses'
export interface StorageSummary {
  name: string
  namespace?: string
  capacity?: string
  access_modes: string[]
  reclaim_policy?: string
  status?: string
  claim?: string
  storage_class?: string
  volume?: string
  provisioner?: string
  binding_mode?: string
  age: string
}
export function createK8sStorageApi(http: AxiosInstance) {
  return {
    list: async (id: number, kind: StorageKind, params: { namespace?: string } = {}) => {
      const result = await http.get<Envelope<{ items: StorageSummary[]; truncated: boolean; continue?: string }>>(`/k8s/clusters/${id}/storage/${kind}`, { params })
      return result.data.data
    },
  }
}
export type K8sStorageApi = ReturnType<typeof createK8sStorageApi>
