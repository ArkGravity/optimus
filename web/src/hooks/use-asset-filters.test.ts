import { defineComponent, h, nextTick, ref } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { useAssetFilters } from './use-asset-filters'
import { pageStateKey } from './usePageState'
import type { CloudAccountSummary } from '@/types/assets'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}
const account = (id: number, provider: 'aws' | 'gcp'): CloudAccountSummary => ({ id, name: `Account ${id}`, provider, project_id: provider === 'gcp' ? 'target' : '', external_account_id: '', scope_revision: 1, cloudkey_id: 2, cloudkey_name: 'key', regions_count: 1, enabled: true, created_at: '', updated_at: '' })
function setup(canReadAccounts = ref(true), list = vi.fn().mockResolvedValue({ items: [], total: 0 }), values: Record<string, unknown> = {}) {
  let filters!: ReturnType<typeof useAssetFilters>
  const state = { values }
  const wrapper = mount(defineComponent({ setup() {
    filters = useAssetFilters({ family: 'instance', canReadAccounts, accountApi: { list } })
    return () => h('div')
  } }), { global: { provide: { [pageStateKey as symbol]: state } } })
  return { filters, wrapper, list, state }
}

describe('shared asset filters', () => {
  it('restores coherent workspace controls and page before installing change invalidation', () => {
    const saved = { provider: 'gcp', account_id: 8, project_id: 'target', region: 'europe-west4', zone: 'europe-west4-a' }
    const { filters, wrapper } = setup(ref(false), undefined, { 'asset-filters.filters': saved, 'asset-filters.page': 4, 'asset-filters.pageSize': 50 })
    expect(filters.filters.value).toEqual(saved)
    expect(filters.page.value).toBe(4)
    expect(filters.pageSize.value).toBe(50)
    filters.setFilters({ region: 'europe-west1' })
    expect(filters.page.value).toBe(1)
    expect(filters.filters.value.zone).toBeUndefined()
    wrapper.unmount()
  })
  it('keeps loading and errors owned by the current generation', async () => {
    const { filters, wrapper } = setup(ref(false))
    let rejectOld!: (reason: Error) => void
    const old = new Promise<string>((_, reject) => { rejectOld = reject })
    const pending = deferred<string>()
    const first = filters.runRequest(() => old, vi.fn())
    const second = filters.runRequest(() => pending.promise, vi.fn())
    expect(filters.loading.value).toBe(true)
    rejectOld(new Error('stale error'))
    await expect(first).resolves.toBe(false)
    expect(filters.loading.value).toBe(true)
    expect(filters.requestError.value).toBeUndefined()
    pending.resolve('latest')
    await second
    expect(filters.loading.value).toBe(false)
    const error = new Error('current failure')
    await expect(filters.runRequest(() => Promise.reject(error), vi.fn())).rejects.toBe(error)
    expect(filters.requestError.value).toBe(error)
    expect(filters.loading.value).toBe(false)
    wrapper.unmount()
  })
  it('resets dependent scope and page without implicitly selecting account or region', async () => {
    const { filters, wrapper } = setup()
    filters.setFilters({ provider: 'aws', account_id: 7, region: 'eu-west-1', zone: 'eu-west-1a', project_id: 'old', vpc_id: 'vpc-1' })
    filters.page.value = 4
    filters.setFilters({ provider: 'gcp' })
    expect(filters.filters.value).toEqual({ provider: 'gcp' })
    expect(filters.page.value).toBe(1)
    filters.setFilters({ account_id: 8, project_id: 'target', region: 'europe-west4', zone: 'europe-west4-a' })
    filters.page.value = 3
    filters.setFilters({ account_id: 9 })
    expect(filters.filters.value).toEqual({ provider: 'gcp', account_id: 9 })
    filters.setFilters({ region: 'custom-new-region', zone: 'custom-new-region-a' })
    filters.setFilters({ region: 'another-new-region' })
    expect(filters.filters.value.zone).toBeUndefined()
    expect(filters.filters.value.region).toBe('another-new-region')
    expect(filters.page.value).toBe(1)
    wrapper.unmount()
  })

  it('never calls account APIs without read permission and retains a numeric account filter', async () => {
    const allowed = ref(false)
    const { filters, list, wrapper } = setup(allowed)
    filters.setFilters({ account_id: 19 })
    await flushPromises()
    expect(list).not.toHaveBeenCalled()
    expect(filters.filters.value.account_id).toBe(19)
    allowed.value = true
    await flushPromises()
    expect(list).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('discards out-of-order account options, revoked permission responses, and unmounted responses', async () => {
    const aws = deferred<{ items: CloudAccountSummary[]; total: number }>()
    const gcp = deferred<{ items: CloudAccountSummary[]; total: number }>()
    const list = vi.fn().mockReturnValueOnce(aws.promise).mockReturnValueOnce(gcp.promise)
    const allowed = ref(true)
    const { filters, wrapper } = setup(allowed, list)
    filters.setFilters({ provider: 'gcp' })
    await nextTick()
    gcp.resolve({ items: [account(8, 'gcp')], total: 1 })
    await flushPromises()
    expect(filters.accountOptions.value).toEqual([{ label: 'Account 8', value: 8 }])
    aws.resolve({ items: [account(7, 'aws')], total: 1 })
    await flushPromises()
    expect(filters.accountOptions.value).toEqual([{ label: 'Account 8', value: 8 }])
    allowed.value = false
    await nextTick()
    expect(filters.accountOptions.value).toEqual([])
    wrapper.unmount()
  })

  it('discards pending metadata on permission revocation and unmount, including pagination', async () => {
    const pending = deferred<{ items: CloudAccountSummary[]; total: number }>()
    const list = vi.fn().mockReturnValue(pending.promise)
    const allowed = ref(true)
    const { filters, wrapper } = setup(allowed, list)
    expect(filters.loadingAccounts.value).toBe(true)
    allowed.value = false
    await nextTick()
    pending.resolve({ items: [account(7, 'aws')], total: 200 })
    await flushPromises()
    expect(list).toHaveBeenCalledOnce()
    expect(filters.accountOptions.value).toEqual([])
    expect(filters.loadingAccounts.value).toBe(false)
    wrapper.unmount()

    const unmounted = deferred<{ items: CloudAccountSummary[]; total: number }>()
    const other = setup(ref(true), vi.fn().mockReturnValue(unmounted.promise))
    other.wrapper.unmount()
    unmounted.resolve({ items: [account(7, 'aws')], total: 200 })
    await flushPromises()
    expect(other.filters.accountOptions.value).toEqual([])
    expect(other.filters.loadingAccounts.value).toBe(false)
    expect(other.list).toHaveBeenCalledOnce()
  })

  it('rejects late list results on newer requests, provider/account/region changes and unmount', async () => {
    const { filters, wrapper, state } = setup(ref(false))
    const apply = vi.fn()
    const old = deferred<string>()
    const current = deferred<string>()
    const first = filters.runRequest(() => old.promise, apply)
    const second = filters.runRequest(() => current.promise, apply)
    current.resolve('current')
    await expect(second).resolves.toBe(true)
    old.resolve('old')
    await expect(first).resolves.toBe(false)
    expect(apply.mock.calls).toEqual([['current']])
    for (const patch of [{ provider: 'gcp' as const }, { account_id: 8 }, { region: 'europe-west4' }]) {
      const pending = deferred<string>()
      const request = filters.runRequest(() => pending.promise, apply)
      filters.setFilters(patch)
      pending.resolve('stale')
      await expect(request).resolves.toBe(false)
    }
    const pending = deferred<string>()
    const request = filters.runRequest(() => pending.promise, apply)
    wrapper.unmount()
    pending.resolve('unmounted')
    await expect(request).resolves.toBe(false)
    expect(apply).toHaveBeenCalledOnce()
    expect(Object.keys(state.values).sort()).toEqual(['asset-filters.filters', 'asset-filters.page', 'asset-filters.pageSize'])
  })
})
