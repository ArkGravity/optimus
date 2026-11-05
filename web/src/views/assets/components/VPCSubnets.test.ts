/* eslint-disable vue/one-component-per-file */
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Input, Pagination } from 'ant-design-vue'
import VPCSubnets from './VPCSubnets.vue'
import type { AssetKey, ListResponse, SubnetSummary } from '@/types/assets'
import { pageStateKey } from '@/hooks/usePageState'
vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const subnet: SubnetSummary = { id: 42, asset_key: 'gcp:subnet:42', provider: 'gcp', cloud_account_id: 7, cloud_account_name: 'account-a', project_id: 'host-project', location_type: 'region', location: 'europe-west4', region: 'europe-west4', zone: '', native_id: '9223372036854775807', native_status: '', network_ref: 'projects/host/global/networks/shared', cidr_blocks: ['10.0.0.0/24', 'fd20::/64'], native_details: { available_ip_count: 0, purpose: 'PRIVATE', private_ip_google_access: false }, subnet_id: '9223372036854775807', vpc_id: '', name: 'subnet-a', availability_zone: '', tags: {}, last_seen_at: '', deleted: false }
const page = defineComponent({ emits: ['change', 'showSizeChange'], setup(_, { emit }) { return () => h('button', { class: 'page', onClick: () => emit('change', 2) }) } })
const search = defineComponent({ emits: ['search', 'change'], setup(_, { emit }) { return () => h('button', { class: 'search', onClick: () => emit('search', 'private') }) } })
const stubs = { 'a-spin': { template: '<div><slot/></div>' }, 'a-alert': { props: ['message'], template: '<p role="alert">{{ message }}</p>' }, 'a-checkbox': true, 'a-pagination': page, 'a-input-search': search }
afterEach(() => { vi.unstubAllGlobals() })
function deferred() { let resolve!: (value: ListResponse<SubnetSummary>) => void; let reject!: (error: Error) => void; const promise = new Promise<ListResponse<SubnetSummary>>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
function setup(listSubnets = vi.fn().mockResolvedValue({ items: [subnet], total: 1 }), key: AssetKey = 'gcp:vpc:42') {
  const state = { values: { 'asset-filters.page': 4, 'asset-filters.filters': { provider: 'gcp', project_id: 'consumer-project' } }, scrollTop: 0, scrollLeft: 0 }
  const wrapper = mount(VPCSubnets, { props: { assetKey: key }, global: { provide: { assetsResourceApi: { listSubnets }, [pageStateKey as symbol]: state }, stubs } })
  return { wrapper, listSubnets, state }
}
describe('parent-scoped subnet details', () => {
  it('applies typed search and clear through the actual Ant Design InputSearch', async () => {
    const listSubnets = vi.fn().mockResolvedValue({ items: [], total: 0 })
    const wrapper = mount(VPCSubnets, { props: { assetKey: 'gcp:vpc:42' }, global: {
      provide: { assetsResourceApi: { listSubnets } },
      components: { AInputSearch: Input.Search },
      stubs: { ...stubs, 'a-input-search': false },
    } })
    await flushPromises()
    const input = wrapper.get('input')
    await input.setValue('private')
    expect(listSubnets).toHaveBeenCalledTimes(1)
    expect(input.element.value).toBe('private')
    await input.trigger('keydown', { key: 'Enter', keyCode: 13 })
    await flushPromises()
    expect(listSubnets).toHaveBeenLastCalledWith('gcp:vpc:42', { page: 1, size: 20, q: 'private' })
    expect(listSubnets).toHaveBeenCalledTimes(2)
    await wrapper.get('.ant-input-clear-icon').trigger('click')
    await flushPromises()
    expect(input.element.value).toBe('')
    expect(listSubnets).toHaveBeenLastCalledWith('gcp:vpc:42', { page: 1, size: 20 })
    expect(listSubnets).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })
  it('resets page once through the actual Ant Design pagination size-selector event chain', async () => {
    vi.stubGlobal('matchMedia', vi.fn((media: string) => ({ matches: false, media, addListener: vi.fn(), removeListener: vi.fn() })))
    const listSubnets = vi.fn().mockResolvedValue({ items: [], total: 200 })
    const wrapper = mount(VPCSubnets, { attachTo: document.body, props: { assetKey: 'gcp:vpc:42' }, global: {
      provide: { assetsResourceApi: { listSubnets } },
      components: { APagination: Pagination }, stubs: { ...stubs, 'a-pagination': false },
    } })
    await flushPromises()
    await wrapper.get('.ant-pagination-item-3').trigger('click')
    await flushPromises()
    expect(listSubnets).toHaveBeenLastCalledWith('gcp:vpc:42', { page: 3, size: 20 })
    listSubnets.mockClear()
    await wrapper.get('.ant-select-selector').trigger('mousedown')
    await flushPromises()
    const option = [...document.body.querySelectorAll<HTMLElement>('.ant-select-item-option')].find(element => element.textContent?.includes('50'))
    expect(option).toBeDefined()
    option!.click()
    await flushPromises()
    expect(listSubnets).toHaveBeenCalledTimes(1)
    expect(listSubnets).toHaveBeenLastCalledWith('gcp:vpc:42', { page: 1, size: 50 })
    expect(wrapper.find('.ant-pagination-item-1').classes()).toContain('ant-pagination-item-active')
    wrapper.unmount()
  })
  it('preserves all CIDRs, native facts and large string identity without reusing list scope/page state', async () => {
    const { wrapper, listSubnets, state } = setup()
    await flushPromises()
    expect(listSubnets).toHaveBeenCalledWith('gcp:vpc:42', { page: 1, size: 20 })
    const rendered = wrapper.find('[data-section="gcp:subnet:42"]')
    for (const value of ['10.0.0.0/24', 'fd20::/64', 'projects/host/global/networks/shared', 'host-project', '9223372036854775807', 'PRIVATE']) expect(rendered.text()).toContain(value)
    expect(rendered.find('[data-field="available_ip_count"] dd').text()).toBe('0')
    expect(rendered.find('[data-field="private_ip_google_access"] dd').text()).toBe('assets.detail.no')
    await wrapper.find('.page').trigger('click')
    await flushPromises()
    expect(listSubnets).toHaveBeenLastCalledWith('gcp:vpc:42', { page: 2, size: 20 })
    await wrapper.find('.search').trigger('click'); await flushPromises()
    expect(listSubnets).toHaveBeenLastCalledWith('gcp:vpc:42', { page: 1, size: 20, q: 'private' })
    expect(state.values['asset-filters.page']).toBe(4)
    expect(Object.keys(state.values)).toHaveLength(2)
    wrapper.unmount()
  })
  it('isolates colliding provider rows and rejects older or unmounted subnet responses', async () => {
    const old = deferred(); const current = deferred()
    const { wrapper, listSubnets } = setup(vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise), 'aws:vpc:42')
    await wrapper.setProps({ assetKey: 'gcp:vpc:42' })
    expect(listSubnets).toHaveBeenLastCalledWith('gcp:vpc:42', { page: 1, size: 20 })
    current.resolve({ items: [subnet], total: 1 }); await flushPromises()
    old.resolve({ items: [{ ...subnet, asset_key: 'aws:subnet:42', name: 'obsolete-aws' }], total: 1 }); await flushPromises()
    expect(wrapper.text()).toContain('subnet-a'); expect(wrapper.text()).not.toContain('obsolete-aws')
    wrapper.unmount()
    const pending = deferred(); const other = setup(vi.fn().mockReturnValue(pending.promise))
    other.wrapper.unmount(); pending.reject(new Error('unmounted')); await flushPromises()
    expect(other.state.values['asset-filters.page']).toBe(4)
  })
  it('clears stale rows on page reload and owns current errors locally', async () => {
    const { wrapper } = setup(vi.fn().mockResolvedValueOnce({ items: [subnet], total: 2 }).mockRejectedValueOnce(new Error('raw provider failure')))
    await flushPromises(); expect(wrapper.text()).toContain('subnet-a')
    await wrapper.find('.page').trigger('click'); await flushPromises()
    expect(wrapper.text()).not.toContain('subnet-a')
    expect(wrapper.find('[role="alert"]').text()).toBe('network.error')
    wrapper.unmount()
  })
})
