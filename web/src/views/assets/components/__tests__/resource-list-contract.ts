import { defineComponent, h, reactive, type Component, type PropType } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Checkbox, Input, InputNumber, Pagination, Select } from 'ant-design-vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { pageStateKey } from '@/hooks/usePageState'
import type { AssetFamily, AssetIdentity } from '@/types/assets'

export const permission = reactive({ allowed: true })
interface Column { key: string; dataIndex?: string }
export const ListTable = defineComponent({
  props: { dataSource: { type: Array as PropType<Array<Record<string, unknown>>>, default: () => [] }, columns: { type: Array as PropType<Column[]>, default: () => [] }, rowKey: { type: String, default: 'id' }, loading: Boolean, customRow: { type: Function, default: undefined } },
  setup(props, { slots }) { return () => h('div', { 'data-loading': String(props.loading) }, props.dataSource.map(record => h('section', { 'data-row': String(record[props.rowKey!]), ...props.customRow?.(record) }, props.columns.map(column => h('div', { 'data-column': column.key }, slots.bodyCell?.({ column, record }) ?? String(record[column.dataIndex!] ?? '')))))) },
})
export const listStubs = {
  'a-card': { template: '<div><slot/></div>' }, 'a-table': ListTable,
  'a-alert': { props: ['message'], template: '<p role="alert">{{ message }}</p>' },
  'a-tag': { template: '<span><slot/></span>' }, PageHeader: { props: ['title'], template: '<h1>{{ title }}</h1>' },
  RouterLink: { props: ['to'], template: '<a :data-to="JSON.stringify(to)"><slot/></a>' },
}
const components = { AInput: Input, AInputSearch: Input.Search, ASelect: Select, AInputNumber: InputNumber, ACheckbox: Checkbox, APagination: Pagination }
export async function choose(wrapper: VueWrapper, field: string, label: string) {
  await wrapper.get(`[data-testid="${field}"] .ant-select-selector`).trigger('mousedown')
  await flushPromises()
  const option = [...document.body.querySelectorAll<HTMLElement>('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option')].find(node => node.textContent?.trim() === label)
  expect(option, `missing ${field} option ${label}`).toBeDefined()
  option!.click()
  await flushPromises()
}
export function pending<T>() { let resolve!: (value: T) => void; let reject!: (error: Error) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no }); return { promise, resolve, reject } }
export function resourceListContract(component: Component, family: Exclude<AssetFamily, 'subnet'>, method: string, fixture: AssetIdentity & { id: number }) {
  describe(`${family} multi-cloud list`, () => {
    const mounted: VueWrapper[] = []
    beforeEach(() => { permission.allowed = true; vi.stubGlobal('matchMedia', vi.fn((media: string) => ({ matches: false, media, addListener: vi.fn(), removeListener: vi.fn() }))) })
    afterEach(() => { mounted.splice(0).forEach(wrapper => wrapper.unmount()); document.body.innerHTML = ''; vi.unstubAllGlobals() })
    function setup(load = vi.fn().mockResolvedValue({ items: [fixture], total: 200 }), state?: { values: Record<string, unknown>; scrollTop: number; scrollLeft: number }) {
      const accountApi = { list: vi.fn().mockResolvedValue({ items: [{ id: 7, name: 'account-seven' }], total: 1 }) }
      const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: { template: '<div />' } }, { path: '/detail/:id', name: `assets.${family === 'vpc' ? 'vpcs' : family === 'database' ? 'databases' : family === 'firewall' ? 'firewalls' : 'instances'}.detail`, component: { template: '<div />' } }] })
      const wrapper = mount(component, { attachTo: document.body, global: { plugins: [router], provide: { assetsResourceApi: { [method]: load }, assetsAccountApi: accountApi, ...(state ? { [pageStateKey as symbol]: state } : {}) }, components, stubs: listStubs } })
      mounted.push(wrapper)
      return { wrapper, load, accountApi, router }
    }
    it('uses exact server keys for colliding provider rows and canonical named detail links', async () => {
      const rows = [{ ...fixture, provider: 'aws', asset_key: `aws:${family}:42` }, { ...fixture, provider: 'gcp', asset_key: `gcp:${family}:42` }, { ...fixture, asset_key: `gcp:${family}:9223372036854775807` }]
      const { wrapper, router } = setup(vi.fn().mockResolvedValue({ items: rows, total: 3 }))
      await flushPromises()
      expect(wrapper.findAll('[data-row]').map(node => node.attributes('data-row'))).toEqual(rows.map(row => row.asset_key))
      for (const row of rows) {
        const link = wrapper.get(`[data-row="${row.asset_key}"] a`)
        expect(JSON.parse(link.attributes('data-to')!)).toEqual({ name: `assets.${family === 'vpc' ? 'vpcs' : family === 'database' ? 'databases' : family === 'firewall' ? 'firewalls' : 'instances'}.detail`, params: { id: row.asset_key } })
      }
      expect(wrapper.text()).toContain(fixture.native_id)
      if (family === 'vpc') {
        for (const record of rows) {
          const row = wrapper.get(`[data-row="${record.asset_key}"]`)
          expect(row.attributes('role')).toBe('link'); expect(row.attributes('tabindex')).toBe('0')
          await row.trigger('click'); await flushPromises()
          expect(router.currentRoute.value.name).toBe('assets.vpcs.detail')
          expect(router.currentRoute.value.params).toEqual({ id: record.asset_key })
          await router.push('/')
          await row.trigger('keydown', { key: 'Enter' }); await flushPromises()
          expect(router.currentRoute.value.name).toBe('assets.vpcs.detail')
          expect(router.currentRoute.value.params).toEqual({ id: record.asset_key })
          await router.push('/')
        }
      }
    })
    it('types a multi-word query and applies real provider/account selectors with dependent resets', async () => {
      const { wrapper, load, accountApi } = setup()
      await flushPromises()
      const input = wrapper.get<HTMLInputElement>('input[data-testid="q"], [data-testid="q"] input')
      for (const letter of 'web server') { await input.setValue(input.element.value + letter); await flushPromises() }
      expect(input.element.value).toBe('web server')
      expect(load).toHaveBeenLastCalledWith({ page: 1, size: 20, q: 'web server' })
      await choose(wrapper, 'provider', 'GCP')
      expect(accountApi.list).toHaveBeenLastCalledWith({ provider: 'gcp', page: 1, size: 100 })
      await choose(wrapper, 'account', 'account-seven')
      await wrapper.get('input[data-testid="region"], [data-testid="region"] input').setValue('future-region')
      await wrapper.get('input[data-testid="zone"], [data-testid="zone"] input').setValue('future-zone')
      expect(load).toHaveBeenLastCalledWith({ page: 1, size: 20, provider: 'gcp', account_id: 7, region: 'future-region', zone: 'future-zone', q: 'web server' })
      await choose(wrapper, 'provider', 'AWS')
      expect(load).toHaveBeenLastCalledWith({ page: 1, size: 20, provider: 'aws', q: 'web server' })
    })
    it('fetches once at page one for the actual Ant pagination size-change chain', async () => {
      const { wrapper, load } = setup()
      await flushPromises()
      await wrapper.get('.ant-pagination-item-3').trigger('click'); await flushPromises()
      expect(load).toHaveBeenLastCalledWith({ page: 3, size: 20 })
      load.mockClear()
      await wrapper.get('.ant-pagination-options .ant-select-selector').trigger('mousedown'); await flushPromises()
      const option = [...document.body.querySelectorAll<HTMLElement>('.ant-select-item-option')].find(node => node.textContent?.includes('50'))
      expect(option).toBeDefined(); option!.click(); await flushPromises()
      expect(load).toHaveBeenCalledTimes(1)
      expect(load).toHaveBeenLastCalledWith({ page: 1, size: 50 })
    })
    it('keeps current rows, loading and errors when obsolete requests finish or fail', async () => {
      const first = pending<{ items: typeof fixture[]; total: number }>(); const second = pending<{ items: typeof fixture[]; total: number }>()
      const { wrapper } = setup(vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise))
      await flushPromises()
      await choose(wrapper, 'provider', 'GCP')
      first.reject(new Error('stale provider secret')); await flushPromises()
      expect(wrapper.find('[role="alert"]').exists()).toBe(false)
      expect(wrapper.get('[data-loading]').attributes('data-loading')).toBe('true')
      second.resolve({ items: [fixture], total: 1 }); await flushPromises()
      expect(wrapper.get('[data-loading]').attributes('data-loading')).toBe('false')
      expect(wrapper.findAll('[data-row]')).toHaveLength(1)
    })
    it('rejects an old page success after the newest page has committed', async () => {
      const oldPage = pending<{ items: typeof fixture[]; total: number }>(); const newestPage = pending<{ items: typeof fixture[]; total: number }>()
      const { wrapper, load } = setup(vi.fn().mockResolvedValueOnce({ items: [fixture], total: 200 }).mockReturnValueOnce(oldPage.promise).mockReturnValueOnce(newestPage.promise))
      await flushPromises()
      wrapper.getComponent(Pagination).vm.$emit('change', 3, 20)
      wrapper.getComponent(Pagination).vm.$emit('change', 4, 20)
      await flushPromises()
      expect(load).toHaveBeenLastCalledWith({ page: 4, size: 20 })
      newestPage.resolve({ items: [{ ...fixture, native_id: 'new-page' }], total: 200 }); await flushPromises()
      oldPage.resolve({ items: [{ ...fixture, native_id: 'old-page' }], total: 3 }); await flushPromises()
      expect(wrapper.text()).toContain('new-page'); expect(wrapper.text()).not.toContain('old-page')
      expect(wrapper.get('[data-loading]').attributes('data-loading')).toBe('false')
      expect(wrapper.getComponent(Pagination).props('total')).toBe(200)
    })
    it('shows safe current errors and suppresses unmounted failures', async () => {
      const { wrapper } = setup(vi.fn().mockRejectedValue(new Error('raw secret')))
      await flushPromises()
      expect(wrapper.get('[role="alert"]').text()).toBe('network.error')
      expect(wrapper.text()).not.toContain('raw secret')
      const later = pending<{ items: typeof fixture[]; total: number }>()
      const other = setup(vi.fn().mockReturnValue(later.promise)); other.wrapper.unmount(); mounted.pop()
      later.reject(new Error('after unmount')); await flushPromises()
    })
    it('reacts to account-read permission and uses numeric fallback without forbidden account reads', async () => {
      permission.allowed = false
      const { wrapper, load, accountApi } = setup()
      await flushPromises()
      expect(accountApi.list).not.toHaveBeenCalled()
      await wrapper.get('[data-testid="account-id"] input, input[data-testid="account-id"]').setValue('9'); await flushPromises()
      expect(load).toHaveBeenLastCalledWith({ page: 1, size: 20, account_id: 9 })
      permission.allowed = true; await flushPromises()
      expect(accountApi.list).toHaveBeenCalledOnce()
      expect(wrapper.find('[data-testid="account"]').exists()).toBe(true)
      permission.allowed = false; await flushPromises()
      expect(wrapper.find('[data-testid="account-id"]').exists()).toBe(true)
    })
    it('renders observed native collections and preserves zero, false and unavailable facts', async () => {
      const { wrapper } = setup()
      await flushPromises()
      const row = wrapper.get('[data-row]')
      if (family === 'instance') {
        for (const value of ['10.0.0.2', '10.0.0.3', 'fd20::1', '203.0.113.1', '2001:db8::1', 'projects/host/global/networks/shared']) expect(row.text()).toContain(value)
      } else if (family === 'vpc') {
        expect(row.get('[data-column="cidr"]').text()).toBe('10.0.0.0/8, fd20::/48')
        expect(row.get('[data-column="is_default"]').text()).toBe('assets.detail.not_applicable')
        expect(row.get('[data-column="location"]').text()).toBe('assets.filters.location.global')
      } else if (family === 'database') {
        expect(row.get('[data-column="endpoint"]').text()).toContain('db.example')
        expect(row.get('[data-column="endpoint"]').text()).toContain('db6.example')
        expect(row.get('[data-column="multi_az"]').text()).toBe('assets.detail.not_applicable')
        expect(row.get('[data-column="publicly_accessible"]').text()).toBe('assets.detail.no')
        expect(row.get('[data-column="storage_gb"]').text()).toBe('0')
      } else {
        expect(row.get('[data-column="inbound_rule_count"]').text()).toBe('1')
        expect(row.get('[data-column="outbound_rule_count"]').text()).toBe('0')
        expect(row.get('[data-column="location"]').text()).toBe('assets.filters.location.global')
      }
    })
    it('renders empty, null and absent non-AWS scalar facts as localized unavailable values', async () => {
      const rows = ['', null, undefined].map((native_status, index) => ({ ...fixture, id: index + 1, asset_key: `gcp:${family}:${index + 1}`, provider: 'gcp', native_status, region: '', zone: '' }))
      const { wrapper } = setup(vi.fn().mockResolvedValue({ items: rows, total: rows.length }))
      await flushPromises()
      for (const row of wrapper.findAll('[data-row]')) {
        for (const field of ['native_status', 'region', 'zone']) expect(row.get(`[data-column="${field}"]`).text()).toBe('assets.detail.not_applicable')
      }
    })
    it('sends family filters with include-deleted and preserves native state independently', async () => {
      const { wrapper, load } = setup(); await flushPromises()
      if (family === 'instance') {
        await choose(wrapper, 'state', 'suspended')
        for (const [field, value] of Object.entries({ ip: 'fd20::1', vpc_id: 'projects/host/global/networks/shared', tag_key: 'label', tag_value: 'value' })) await wrapper.get(`input[data-testid="${field}"], [data-testid="${field}"] input`).setValue(value)
        expect(load.mock.lastCall?.[0]).toMatchObject({ state: 'suspended', ip: 'fd20::1', vpc_id: 'projects/host/global/networks/shared', tag_key: 'label', tag_value: 'value' })
      } else if (family === 'database') {
        await wrapper.get('input[data-testid="engine"], [data-testid="engine"] input').setValue('POSTGRES')
        await wrapper.get('input[data-testid="status"], [data-testid="status"] input').setValue('RUNNABLE')
        expect(load.mock.lastCall?.[0]).toMatchObject({ engine: 'POSTGRES', status: 'RUNNABLE' })
      } else if (family === 'firewall') {
        await wrapper.get('input[data-testid="network_ref"], [data-testid="network_ref"] input').setValue('projects/host/global/networks/shared')
        expect(load.mock.lastCall?.[0]).toMatchObject({ network_ref: 'projects/host/global/networks/shared' })
      }
      await wrapper.get('input[type="checkbox"]').setValue(true); await flushPromises()
      expect(load.mock.lastCall?.[0]).toMatchObject({ include_deleted: true })
      expect(load.mock.lastCall?.[0]).not.toHaveProperty('location_type')
      expect(wrapper.text()).toContain(fixture.native_status)
    })
    it('restores only controls and refetches on return without persisting rows or options', async () => {
      const state = { values: { 'asset-filters.page': 3, 'asset-filters.pageSize': 50, 'asset-filters.filters': { provider: 'gcp', q: 'saved filter' } }, scrollTop: 0, scrollLeft: 0 }
      const first = setup(undefined, state); await flushPromises()
      expect(first.load).toHaveBeenCalledWith({ page: 3, size: 50, provider: 'gcp', q: 'saved filter' })
      first.wrapper.unmount(); mounted.pop()
      const second = setup(undefined, state); await flushPromises()
      expect(second.load).toHaveBeenCalledOnce()
      expect(Object.keys(state.values).sort()).toEqual(['asset-filters.filters', 'asset-filters.page', 'asset-filters.pageSize'])
    })
  })
}
