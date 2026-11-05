import { defineComponent, h } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Input, InputNumber, Pagination, Select } from 'ant-design-vue'
import List from '../List.vue'
import { choose, listStubs, pending, permission } from '../../components/__tests__/resource-list-contract'
import type { SyncRunSummary } from '@/types/assets'
vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/hooks/usePermission', () => ({ usePermission: () => ({ has: () => permission.allowed }) }))
const datePicker = defineComponent({ emits: ['change'], setup(_, { emit }) { return () => h('button', { class: 'started-after', onClick: () => emit('change', { toDate: () => new Date('2026-07-15T12:34:56.000Z') }) }) } })
const row: SyncRunSummary = { id: 42, provider: 'gcp', cloud_account_id: 9, location_type: 'global', location: '', scope_revision: 3, region: '', resource_type: 'planning', started_at: '2026-07-15T12:00:00Z', finished_at: '2026-07-15T12:00:01.500Z', status: 'failed', items_seen: 0, items_softdeleted: 0, error: 'Asset collection failed', error_code: 50001, trigger: 'cron' }
describe('multi-cloud sync run inventory', () => {
  const mounted: VueWrapper[] = []
  beforeEach(() => { permission.allowed = true; vi.stubGlobal('matchMedia', vi.fn((media: string) => ({ matches: false, media, addListener: vi.fn(), removeListener: vi.fn() }))) })
  afterEach(() => { mounted.splice(0).forEach(wrapper => wrapper.unmount()); document.body.innerHTML = ''; vi.unstubAllGlobals() })
  function setup(load = vi.fn().mockResolvedValue({ items: [row], total: 200 })) {
    const accountApi = { list: vi.fn().mockResolvedValue({ items: [{ id: 9, name: 'account-nine' }], total: 1 }) }
    const wrapper = mount(List, { attachTo: document.body, global: { provide: { assetsSyncApi: { listRuns: load }, assetsAccountApi: accountApi }, components: { AInput: Input, AInputNumber: InputNumber, ASelect: Select, APagination: Pagination }, stubs: { ...listStubs, 'a-date-picker': datePicker, 'a-tooltip': { props: ['title'], template: '<span class="tooltip" :data-title="title"><slot/></span>' } } } })
    mounted.push(wrapper); return { wrapper, load, accountApi }
  }
  it('renders provider, truthful global scope, deleted-account numeric fallback, statuses and fixed error codes', async () => {
    const { wrapper } = setup(vi.fn().mockResolvedValue({ items: ['running', 'success', 'failed', 'skipped'].map((status, index) => ({ ...row, id: 42 + index, status })), total: 4 }))
    await flushPromises()
    expect(wrapper.findAll('[data-row]').map(node => node.attributes('data-row'))).toEqual(['42', '43', '44', '45'])
    for (const value of ['gcp', 'assets.filters.location.global', 'assets.sync.resource_types.planning', 'assets.sync.run_status.running', 'assets.sync.run_status.success', 'assets.sync.run_status.failed', 'assets.sync.run_status.skipped', '50001', '1.5s']) expect(wrapper.text()).toContain(value)
    expect(wrapper.get('[data-column="account"]').text()).toBe('9')
    expect(wrapper.get('.tooltip').attributes('data-title')).toBe(row.error)
    expect(wrapper.text()).not.toContain(row.error)
  })
  it('sends only supported sync provider/location/account/unit/status/time/paging filters', async () => {
    const { wrapper, load, accountApi } = setup(); await flushPromises()
    await choose(wrapper, 'provider', 'GCP'); await choose(wrapper, 'account', 'account-nine')
    expect(accountApi.list).toHaveBeenLastCalledWith({ provider: 'gcp', page: 1, size: 100 })
    await choose(wrapper, 'location-type', 'assets.filters.location.zone')
    await wrapper.get('input[data-testid="location"], [data-testid="location"] input').setValue('europe-west4-a')
    await choose(wrapper, 'resource-type', 'assets.sync.resource_types.subnet')
    await choose(wrapper, 'status', 'assets.sync.run_status.failed')
    await wrapper.get('.started-after').trigger('click'); await flushPromises()
    expect(load).toHaveBeenLastCalledWith({ page: 1, size: 20, provider: 'gcp', account_id: 9, location_type: 'zone', location: 'europe-west4-a', resource_type: 'subnet', status: 'failed', started_after: '2026-07-15T12:34:56.000Z' })
    await wrapper.get('.ant-pagination-item-3').trigger('click'); await flushPromises()
    expect(load.mock.lastCall?.[0]).toMatchObject({ page: 3 })
    await choose(wrapper, 'provider', 'AWS')
    expect(load.mock.lastCall?.[0]).toEqual({ page: 1, size: 20, provider: 'aws', resource_type: 'subnet', status: 'failed', started_after: '2026-07-15T12:34:56.000Z' })
  })
  it('uses numeric account input for resource-only permission and rejects rounded identifiers', async () => {
    permission.allowed = false
    const { wrapper, load, accountApi } = setup(); await flushPromises()
    expect(accountApi.list).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="account-id"] input, input[data-testid="account-id"]').setValue('9'); await flushPromises()
    expect(load).toHaveBeenLastCalledWith({ page: 1, size: 20, account_id: 9 })
    await wrapper.get('[data-testid="account-id"] input, input[data-testid="account-id"]').setValue('9007199254740993'); await flushPromises()
    expect(load).toHaveBeenLastCalledWith({ page: 1, size: 20 })
  })
  it('handles the actual pagination size-change chain with exactly one page-one request', async () => {
    const { wrapper, load } = setup(); await flushPromises()
    await wrapper.get('.ant-pagination-item-3').trigger('click'); await flushPromises(); load.mockClear()
    await wrapper.get('.ant-pagination-options .ant-select-selector').trigger('mousedown'); await flushPromises()
    const option = [...document.body.querySelectorAll<HTMLElement>('.ant-select-item-option')].find(node => node.textContent?.includes('50'))
    expect(option).toBeDefined(); option!.click(); await flushPromises()
    expect(load).toHaveBeenCalledTimes(1)
    expect(load).toHaveBeenLastCalledWith({ page: 1, size: 50 })
  })
  it('discards obsolete failures, shows safe current errors, and handles unmount', async () => {
    const first = pending<{ items: SyncRunSummary[]; total: number }>(); const second = pending<{ items: SyncRunSummary[]; total: number }>()
    const { wrapper } = setup(vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)); await flushPromises()
    await choose(wrapper, 'provider', 'GCP')
    first.reject(new Error('obsolete secret')); await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.get('[data-loading]').attributes('data-loading')).toBe('true')
    second.reject(new Error('current secret')); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('network.error')
    expect(wrapper.text()).not.toContain('secret')
    const last = pending<{ items: SyncRunSummary[]; total: number }>(); const other = setup(vi.fn().mockReturnValue(last.promise)); other.wrapper.unmount(); mounted.pop(); last.reject(new Error('unmounted')); await flushPromises()
  })
})
