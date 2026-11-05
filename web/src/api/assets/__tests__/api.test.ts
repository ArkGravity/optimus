import type { AxiosInstance } from 'axios'
import { describe, expect, it, vi } from 'vitest'

import { makeAssetsAccountApi } from '../account'
import { makeAssetsResourceApi } from '../resource'
import { makeAssetsSyncApi } from '../sync'

function envelope<T>(data: T) {
  return { data: { code: 0, data, message: 'ok' } }
}

function makeClient() {
  const methods = {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn(),
  }
  return { methods, client: methods as unknown as AxiosInstance }
}

describe('assets API factories', () => {
  it('preserves canonical provider keys, including row-ID collisions and unsafe numeric IDs', async () => {
    const { methods, client } = makeClient()
    const api = makeAssetsResourceApi(client)
    methods.get.mockResolvedValue(envelope({}))
    await api.getInstance('aws:instance:19')
    expect(methods.get).toHaveBeenLastCalledWith('/assets/instances/aws%3Ainstance%3A19')
    await api.getInstance('gcp:instance:19', { include_deleted: true })
    expect(methods.get).toHaveBeenLastCalledWith('/assets/instances/gcp%3Ainstance%3A19', { params: { include_deleted: true } })
    await api.getVPC('gcp:vpc:9007199254740993')
    expect(methods.get).toHaveBeenLastCalledWith('/assets/vpcs/gcp%3Avpc%3A9007199254740993')
    await api.getDatabase(9)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/databases/9')
    await api.getFirewall('aliyun:firewall:19')
    expect(methods.get).toHaveBeenLastCalledWith('/assets/firewalls/aliyun%3Afirewall%3A19')
    await api.listSubnets('gcp:vpc:19', { include_deleted: true, page: 2, size: 50 })
    expect(methods.get).toHaveBeenLastCalledWith('/assets/vpcs/gcp%3Avpc%3A19/subnets', { params: { include_deleted: true, page: 2, size: 50 } })
  })

  it('forwards multi-cloud filters and authenticated region options without renaming wire fields', async () => {
    const { methods, client } = makeClient()
    methods.get.mockResolvedValue(envelope({ items: [], total: 0 }))
    const api = makeAssetsResourceApi(client)
    const scope = { provider: 'gcp' as const, account_id: 7, project_id: 'target', region: 'europe-west4', zone: 'europe-west4-a', q: 'core', include_deleted: true, page: 2, size: 10 }
    const instance = { ...scope, state: 'running', vpc_id: 'projects/host/global/networks/core', ip: '2001:db8::1', tag_key: 'env', tag_value: 'prod' }
    await api.listInstances(instance)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/instances', { params: instance })
    const firewall = { ...scope, network_ref: 'projects/host/global/networks/core' }
    await api.listFirewalls(firewall)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/firewalls', { params: firewall })
    const database = { ...scope, engine: 'POSTGRES', status: 'RUNNABLE' }
    await api.listDatabases(database)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/databases', { params: database })
    await api.listVPCs(scope)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/vpcs', { params: scope })
    const options = { provider: 'gcp' as const, cloudkey_id: 4, project_id: 'target' }
    await makeAssetsAccountApi(client).regionOptions(options)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/cloud-accounts/region-options', { params: options })
    const runs = { provider: 'gcp' as const, location_type: 'global' as const, location: 'global', resource_type: 'firewall' as const }
    await makeAssetsSyncApi(client).listRuns(runs)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/sync-runs', { params: runs })
  })
  it('uses cloud-account paths, forwards snake_case queries, and unwraps envelopes', async () => {
    const { methods, client } = makeClient()
    const api = makeAssetsAccountApi(client)
    const params = { q: 'prod', include_deleted: true, page: 2, size: 50 }
    const result = { items: [], total: 0 }
    methods.get.mockResolvedValueOnce(envelope(result))

    await expect(api.list(params)).resolves.toBe(result)
    expect(methods.get).toHaveBeenCalledWith('/assets/cloud-accounts', { params })

    methods.get.mockResolvedValueOnce(envelope({ id: 7 }))
    await api.get(7)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/cloud-accounts/7')

    const create = {
      name: 'prod', provider: 'aws' as const, cloudkey_id: 3, enabled_regions: ['eu-west-1'],
    }
    methods.post.mockResolvedValueOnce(envelope({ id: 7 }))
    await api.create(create)
    expect(methods.post).toHaveBeenCalledWith('/assets/cloud-accounts', create)

    const update = { enabled_regions: ['eu-central-1'], enabled: false }
    methods.put.mockResolvedValueOnce(envelope({ id: 7 }))
    await api.update(7, update)
    expect(methods.put).toHaveBeenCalledWith('/assets/cloud-accounts/7', update)

    methods.delete.mockResolvedValueOnce(envelope({ cascaded_resources_count: 4 }))
    await expect(api.remove(7)).resolves.toEqual({ cascaded_resources_count: 4 })
    expect(methods.delete).toHaveBeenCalledWith('/assets/cloud-accounts/7')

    methods.post.mockResolvedValueOnce(envelope({ queued: true, started_at: '2026-07-16T00:00:00Z' }))
    await expect(api.triggerSync(7)).resolves.toMatchObject({ queued: true })
    expect(methods.post).toHaveBeenLastCalledWith('/assets/cloud-accounts/7/sync')
  })

  it('uses resource paths and forwards each resource query unchanged', async () => {
    const { methods, client } = makeClient()
    const api = makeAssetsResourceApi(client)
    methods.get.mockResolvedValue(envelope({ items: [], total: 0 }))

    const instances = { account_id: 2, region: 'eu-west-1', vpc_id: 'vpc-1', include_deleted: true }
    await api.listInstances(instances)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/instances', { params: instances })

    const vpcs = { q: 'core', page: 3, size: 10 }
    await api.listVPCs(vpcs)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/vpcs', { params: vpcs })

    const groups = { account_id: 2, region: 'eu-west-1', vpc_id: 'vpc-1', include_deleted: true }
    await api.listSecurityGroups(groups)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/security-groups', { params: groups })
    await api.getSecurityGroup(19)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/security-groups/19')

    await api.listSubnets(9)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/vpcs/9/subnets')

    const databases = { engine: 'postgres', status: 'available', page: 1, size: 20 }
    await api.listDatabases(databases)
    expect(methods.get).toHaveBeenLastCalledWith('/assets/databases', { params: databases })
  })

  it('uses the sync-runs path and preserves started_after', async () => {
    const { methods, client } = makeClient()
    const api = makeAssetsSyncApi(client)
    const params = {
      account_id: 2,
      resource_type: 'network' as const,
      status: 'failed' as const,
      started_after: '2026-07-01T00:00:00Z',
      page: 2,
      size: 25,
    }
    const result = { items: [], total: 0 }
    methods.get.mockResolvedValueOnce(envelope(result))

    await expect(api.listRuns(params)).resolves.toBe(result)
    expect(methods.get).toHaveBeenCalledWith('/assets/sync-runs', { params })
  })
})
