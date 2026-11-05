import type { AxiosInstance } from 'axios'

import type { Envelope } from '@/types/api'
import type {
  SecurityGroupSummary,
  SecurityGroupDetail,
  DatabaseSummary,
  InstanceSummary,
  ListResponse,
  SubnetSummary,
  VPCSummary,
  AssetKey,
  Provider,
  FirewallSummary,
  FirewallDetail,
} from '@/types/assets'

export interface ResourceListParams {
  provider?: Provider
  project_id?: string
  zone?: string
  account_id?: number
  region?: string
  q?: string
  include_deleted?: boolean
  page?: number
  size?: number
}

export interface SecurityGroupListParams extends ResourceListParams { vpc_id?: string }

export interface InstanceListParams extends ResourceListParams {
  ip?: string
  tag_key?: string
  tag_value?: string
  state?: string
  vpc_id?: string
}

export interface FirewallListParams extends ResourceListParams { network_ref?: string }
export interface ResourceDetailParams { include_deleted?: boolean }

export interface DatabaseListParams extends ResourceListParams {
  engine?: string
  status?: string
}

export interface SubnetListParams {
  q?: string
  include_deleted?: boolean
  page?: number
  size?: number
}

export function makeAssetsResourceApi(client: AxiosInstance) {
  async function get<T>(family: string, key: AssetKey | number, params?: ResourceDetailParams): Promise<T> {
    const path = `/assets/${family}/${encodeURIComponent(String(key))}`
    const response = params
      ? await client.get<Envelope<T>>(path, { params })
      : await client.get<Envelope<T>>(path)
    return response.data.data
  }
  return {
    getInstance: (key: AssetKey | number, params?: ResourceDetailParams) => get<InstanceSummary>('instances', key, params),
    getVPC: (key: AssetKey | number, params?: ResourceDetailParams) => get<VPCSummary>('vpcs', key, params),
    getDatabase: (key: AssetKey | number, params?: ResourceDetailParams) => get<DatabaseSummary>('databases', key, params),
    getFirewall: (key: AssetKey | number, params?: ResourceDetailParams) => get<FirewallDetail>('firewalls', key, params),
    listFirewalls: async (params: FirewallListParams = {}) => {
      const response = await client.get<Envelope<ListResponse<FirewallSummary>>>('/assets/firewalls', { params })
      return response.data.data
    },
    listSecurityGroups: async (params: SecurityGroupListParams = {}) => {
      const response = await client.get<Envelope<ListResponse<SecurityGroupSummary>>>('/assets/security-groups', { params })
      return response.data.data
    },
    getSecurityGroup: async (id: number) => {
      const response = await client.get<Envelope<SecurityGroupDetail>>(`/assets/security-groups/${id}`)
      return response.data.data
    },
    listInstances: async (params: InstanceListParams = {}) => {
      const response = await client.get<Envelope<ListResponse<InstanceSummary>>>('/assets/instances', { params })
      return response.data.data
    },
    listVPCs: async (params: ResourceListParams = {}) => {
      const response = await client.get<Envelope<ListResponse<VPCSummary>>>('/assets/vpcs', { params })
      return response.data.data
    },
    listSubnets: async (vpcRowID: AssetKey | number, params?: SubnetListParams) => {
      const path = `/assets/vpcs/${encodeURIComponent(String(vpcRowID))}/subnets`
      const response = params
        ? await client.get<Envelope<ListResponse<SubnetSummary>>>(path, { params })
        : await client.get<Envelope<ListResponse<SubnetSummary>>>(path)
      return response.data.data
    },
    listDatabases: async (params: DatabaseListParams = {}) => {
      const response = await client.get<Envelope<ListResponse<DatabaseSummary>>>('/assets/databases', { params })
      return response.data.data
    },
  }
}

export type AssetsResourceApi = ReturnType<typeof makeAssetsResourceApi>
