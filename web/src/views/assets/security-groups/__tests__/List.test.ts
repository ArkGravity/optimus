/* eslint-disable vue/require-prop-types */
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import List from '../List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const has = vi.fn(() => true)
vi.mock('@/hooks/usePermission', () => ({ usePermission: () => ({ has }) }))
vi.mock('ant-design-vue', () => ({ message: { error: vi.fn() } }))
const ATable = defineComponent({
  props: ['dataSource', 'customRow'],
  setup(props) { return () => h('div', (props.dataSource ?? []).map((row: { id: number }) => h('button', { class: 'row', ...props.customRow?.(row) }, String(row.id)))) },
})
const stubs = {
  'a-card': { template: '<div><slot/></div>' }, 'a-table': ATable,
  'a-input-search': true, 'a-select': true, 'a-input': true, 'a-checkbox': true, 'a-input-number': true,
  'a-pagination': true, 'a-tag': { template: '<span><slot/></span>' }, PageHeader: true,
  'a-button': true,
  'a-drawer': { name: 'TestDrawer', props: ['open'], template: '<div v-if="open" class="drawer"><slot/></div>' },
  'a-spin': { template: '<div><slot/></div>' }, 'a-descriptions': { template: '<div><slot/></div>' },
  'a-descriptions-item': { template: '<span><slot/></span>' }, 'a-tabs': { template: '<div><slot/></div>' },
  'a-tab-pane': { template: '<div><slot/></div>' },
}
const summary = { id: 19, group_id: 'sg-1' }
const detail = { ...summary, group_name: 'web', tags: {}, inbound_rules: [], outbound_rules: [] }
afterEach(() => { has.mockReturnValue(true) })

describe('Security Groups list', () => {
  it('loads pagination, applies VPC filters and opens rule details', async () => {
    const api = { listSecurityGroups: vi.fn().mockResolvedValue({ items: [summary], total: 1 }), getSecurityGroup: vi.fn().mockResolvedValue(detail) }
    const accounts = { list: vi.fn().mockResolvedValue({ items: [], total: 0 }) }
    const wrapper = mount(List, { global: { provide: { assetsResourceApi: api, assetsAccountApi: accounts }, stubs } })
    await flushPromises()
    expect(api.listSecurityGroups).toHaveBeenCalledWith({ page: 1, size: 20 })
    const vpcInput = wrapper.findAllComponents({ name: 'a-input' })
    expect(vpcInput).toHaveLength(2)
    vpcInput[1]!.vm.$emit('update:value', ' vpc-1 ')
    vpcInput[1]!.vm.$emit('press-enter')
    await flushPromises()
    expect(api.listSecurityGroups).toHaveBeenLastCalledWith({ page: 1, size: 20, vpc_id: 'vpc-1' })
    await wrapper.find('.row').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(api.getSecurityGroup).toHaveBeenCalledWith(19)
    expect(wrapper.find('.drawer').text()).toContain('web')
    wrapper.unmount()
  })
  it('does not fetch account names without permission or restore late drawer responses', async () => {
    has.mockReturnValue(false)
    let resolve!: (value: typeof detail) => void
    const api = { listSecurityGroups: vi.fn().mockResolvedValue({ items: [summary], total: 1 }), getSecurityGroup: vi.fn().mockReturnValue(new Promise(r => { resolve = r })) }
    const accounts = { list: vi.fn() }
    const wrapper = mount(List, { global: { provide: { assetsResourceApi: api, assetsAccountApi: accounts }, stubs } })
    await flushPromises()
    expect(accounts.list).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="account-id"]').exists()).toBe(true)
    await wrapper.find('.row').trigger('click')
    wrapper.findComponent({ name: 'TestDrawer' }).vm.$emit('close')
    resolve(detail)
    await flushPromises()
    expect(wrapper.find('.drawer').exists()).toBe(false)
    wrapper.unmount()
  })
})
