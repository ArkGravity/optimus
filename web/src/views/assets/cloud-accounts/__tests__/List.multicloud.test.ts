import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { Input, Select, Pagination, message } from 'ant-design-vue'
import type * as AntDesignVue from 'ant-design-vue'
import { useAuthStore } from '@/stores/auth'
import { permissionDirective } from '@/directives/permission'
import { choose, ListTable, pending } from '../../components/__tests__/resource-list-contract'
import List from '../List.vue'
vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('ant-design-vue', async original => ({ ...await original<typeof AntDesignVue>(), message: { success: vi.fn(), error: vi.fn() } }))
const rows = [{ id: 3, name: 'project account', provider: 'gcp', project_id: 'target-project', external_account_id: 'external-identity', scope_revision: 4, cloudkey_name: 'gcp-key', regions_count: 2, enabled: true }]
const mounted: VueWrapper[] = []
function setup() {
  const api = { list: vi.fn().mockResolvedValue({ items: rows, total: 200 }), get: vi.fn(), triggerSync: vi.fn(), remove: vi.fn() }
  const wrapper = mount(List, { attachTo: document.body, global: { provide: { assetsAccountApi: api }, components: { AInputSearch: Input.Search, ASelect: Select, APagination: Pagination }, directives: { permission: permissionDirective }, stubs: {
    'a-card': { template: '<div><slot/></div>' }, 'a-tag': { template: '<span><slot/></span>' }, 'a-space': { template: '<span><slot/></span>' }, 'a-button': { template: '<button><slot/></button>' }, 'a-popconfirm': { template: '<span><slot/></span>' }, 'a-table': ListTable, PageHeader: { template: '<div><slot/></div>' }, Form: { name: 'AccountFormStub', props: ['open', 'editing'], template: '<div />' },
  } } }); mounted.push(wrapper); return { wrapper, api }
}
describe('cloud account multi-cloud controls', () => {
  beforeEach(() => { vi.clearAllMocks(); setActivePinia(createPinia()); useAuthStore().setPermissions(['assets:account:write']); vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addListener: vi.fn(), removeListener: vi.fn() }))) })
  afterEach(() => { mounted.splice(0).forEach(w => w.unmount()); document.body.innerHTML = ''; vi.unstubAllGlobals() })
  it('renders provider/project/identity/scope and combines real search/provider/pagination events', async () => {
    const { wrapper, api } = setup(); await flushPromises()
    for (const value of ['GCP', 'target-project', 'external-identity', '4']) expect(wrapper.text()).toContain(value)
    await wrapper.get('input.ant-input').setValue('project account'); await wrapper.get('input.ant-input').trigger('keydown', { key: 'Enter', keyCode: 13 }); await flushPromises()
    await choose(wrapper, 'provider', 'GCP'); expect(api.list).toHaveBeenLastCalledWith({ page: 1, size: 20, q: 'project account', provider: 'gcp' })
    await wrapper.get('.ant-pagination-item-3').trigger('click'); await flushPromises(); api.list.mockClear()
    await wrapper.get('.ant-pagination-options .ant-select-selector').trigger('mousedown'); await flushPromises()
    const option = [...document.body.querySelectorAll<HTMLElement>('.ant-select-item-option')].find(node => node.textContent?.includes('50'))
    expect(option).toBeDefined(); option!.click(); await flushPromises()
    expect(api.list).toHaveBeenCalledTimes(1); expect(api.list).toHaveBeenLastCalledWith({ page: 1, size: 50, q: 'project account', provider: 'gcp' })
  })
  it('does not open a stale edit response over a later create selection', async () => {
    const { wrapper, api } = setup(); await flushPromises(); const old = pending<object>(); api.get.mockReturnValueOnce(old.promise)
    await wrapper.get('[data-testid="edit-3"]').trigger('click'); await wrapper.get('[data-testid="create-account"]').trigger('click')
    old.resolve({ ...rows[0], enabled_regions: ['us-central1'] }); await flushPromises()
    expect(wrapper.getComponent({ name: 'AccountFormStub' }).props('editing')).toBeNull()
  })
  it('keeps current loading/error ownership after stale failures and unmount', async () => {
    const { wrapper, api } = setup(); await flushPromises()
    const old = pending<object>(); const newest = pending<object>()
    api.list.mockReturnValueOnce(old.promise).mockReturnValueOnce(newest.promise)
    await choose(wrapper, 'provider', 'AWS'); await choose(wrapper, 'provider', 'GCP')
    old.reject(new Error('stale private error')); await flushPromises()
    expect(message.error).not.toHaveBeenCalled()
    expect(wrapper.get('[data-loading]').attributes('data-loading')).toBe('true')
    newest.reject(new Error('current private error')); await flushPromises()
    expect(message.error).toHaveBeenCalledWith('network.error')
    expect(wrapper.get('[data-loading]').attributes('data-loading')).toBe('false')
    vi.mocked(message.error).mockClear()
    const afterUnmount = pending<object>(); api.list.mockReturnValueOnce(afterUnmount.promise)
    await choose(wrapper, 'provider', 'AWS'); wrapper.unmount(); mounted.pop()
    afterUnmount.reject(new Error('unmounted private error')); await flushPromises()
    expect(message.error).not.toHaveBeenCalled()
  })
  it('REVIEW rejects old account list success after a new provider selection commits', async () => {
    const { wrapper, api } = setup(); await flushPromises()
    const old = pending<object>(); const newest = pending<object>()
    api.list.mockReturnValueOnce(old.promise).mockReturnValueOnce(newest.promise)
    await choose(wrapper, 'provider', 'AWS'); await choose(wrapper, 'provider', 'GCP')
    newest.resolve({ items: [{ ...rows[0], name: 'new-gcp-account' }], total: 200 }); await flushPromises()
    expect(wrapper.text()).toContain('new-gcp-account')
    old.resolve({ items: [{ ...rows[0], provider: 'aws', name: 'old-aws-account' }], total: 1 }); await flushPromises()
    expect(wrapper.text()).toContain('new-gcp-account')
    expect(wrapper.text()).not.toContain('old-aws-account')
    expect(wrapper.getComponent(Pagination).props('total')).toBe(200)
  })
})
