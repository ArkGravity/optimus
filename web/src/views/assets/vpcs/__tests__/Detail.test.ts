/* eslint-disable vue/one-component-per-file */
import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import Detail from '../Detail.vue'
import { vpc } from '../../components/__tests__/fixtures'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/hooks/usePermission', () => ({ usePermission: () => ({ has: () => true }) }))
const { showError } = vi.hoisted(() => ({ showError: vi.fn() }))
vi.mock('ant-design-vue', () => ({ message: { error: showError } }))
let routeID: string | string[] = '12'
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { get id() { return routeID } } }),
  useRouter: () => ({ back: vi.fn() }),
}))

const AInputSearch = defineComponent({ name: 'AInputSearch', emits: ['search'], setup: (_, { emit }) => () => h('button', { class: 'search', onClick: () => emit('search', 'private') }) })
const APagination = defineComponent({ name: 'APagination', emits: ['change'], setup: (_, { emit }) => () => h('button', { class: 'page', onClick: () => emit('change', 2) }) })
const stubs = {
  'a-card': { template: '<div><slot/></div>' }, 'a-table': { template: '<div />' },
  'a-input-search': AInputSearch, 'a-checkbox': true, 'a-pagination': APagination,
  'a-button': true, 'a-tag': true,
  'a-spin': { template: '<div><slot/></div>' },
  'a-alert': { props: ['message'], template: '<p role="alert">{{ message }}</p>' },
  PageHeader: true,
}

describe('Assets VPC detail', () => {
  beforeEach(() => { routeID = '12'; showError.mockClear() })

  it('loads the native VPC detail and scopes subnets to the returned server key', async () => {
    routeID = 'gcp:vpc:42'
    const api = {
      getVPC: vi.fn().mockResolvedValue({ asset_key: 'gcp:vpc:42', provider: 'gcp', project_id: 'host-project', location_type: 'global', location: '', zone: '', native_id: '9223372036854775807', native_status: '', cloud_account_id: 7, name: 'shared-network', tags: {}, cidr_blocks: [], network_ref: 'projects/host-project/global/networks/shared-network', native_details: { auto_create_subnetworks: false, mtu: 0 }, is_default: null }),
      listSubnets: vi.fn().mockResolvedValue({ items: [], total: 0 }),
    }
    const wrapper = mount(Detail, { global: { provide: { assetsResourceApi: api }, stubs } })
    await vi.waitFor(() => expect(api.getVPC).toHaveBeenCalledWith('gcp:vpc:42', { include_deleted: true }))
    await vi.waitFor(() => expect(api.listSubnets).toHaveBeenCalledWith('gcp:vpc:42', { page: 1, size: 20 }))
    expect(wrapper.text()).toContain('9223372036854775807')
    expect(wrapper.text()).toContain('projects/host-project/global/networks/shared-network')
    expect(wrapper.text()).toContain('assets.detail.not_applicable')
    wrapper.unmount()
  })

  it('preserves numeric AWS detail compatibility and paginates/filters using the returned canonical key', async () => {
    const api = { getVPC: vi.fn().mockResolvedValue({ ...vpc, asset_key: 'aws:vpc:12', provider: 'aws' }), listSubnets: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
    const wrapper = mount(Detail, { global: { provide: { assetsResourceApi: api }, stubs } })
    await vi.waitFor(() => expect(api.getVPC).toHaveBeenCalledWith(12, { include_deleted: true }))
    await vi.waitFor(() => expect(api.listSubnets).toHaveBeenCalledWith('aws:vpc:12', { page: 1, size: 20 }))
    await wrapper.find('.search').trigger('click')
    await vi.waitFor(() => expect(api.listSubnets).toHaveBeenLastCalledWith('aws:vpc:12', { page: 1, size: 20, q: 'private' }))
    await wrapper.find('.page').trigger('click')
    await vi.waitFor(() => expect(api.listSubnets).toHaveBeenLastCalledWith('aws:vpc:12', { page: 2, size: 20, q: 'private' }))
    wrapper.unmount()
  })

  it('rejects invalid IDs without calling the API', async () => {
    routeID = '12oops'
    const api = { getVPC: vi.fn(), listSubnets: vi.fn() }
    const wrapper = mount(Detail, { global: { provide: { assetsResourceApi: api }, stubs } })
    expect(wrapper.find('[role="alert"]').text()).toBe('assets.detail.invalid_key')
    expect(api.getVPC).not.toHaveBeenCalled()
    expect(api.listSubnets).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
