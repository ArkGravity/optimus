/* eslint-disable vue/one-component-per-file */
import { defineComponent, h, nextTick, ref } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { Input, Select, InputNumber, Checkbox } from 'ant-design-vue'
import AssetFilters from './AssetFilters.vue'
import { useAssetFilters, type AssetFiltersController } from '@/hooks/use-asset-filters'
import type { AssetFamily, LocationType } from '@/types/assets'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const control = defineComponent({
  props: { value: { type: [String, Number], default: undefined }, options: { type: Array, default: () => [] } },
  emits: ['change'],
  setup(props, { attrs, emit }) { return () => h('input', { ...attrs, value: props.value, onChange: (event: Event) => emit('change', (event.target as HTMLInputElement).value) }) },
})
const textControl = defineComponent({
  props: { value: { type: String, default: undefined } }, emits: ['change'],
  setup(props, { attrs, emit }) { return () => h('input', { ...attrs, value: props.value, onChange: (event: Event) => emit('change', event) }) },
})
function setup(family: AssetFamily = 'instance', allowed = ref(false), locationType?: LocationType) {
  const list = vi.fn().mockResolvedValue({ items: [], total: 0 })
  const change = vi.fn()
  let controller!: AssetFiltersController
  const wrapper = mount(defineComponent({ setup() {
    controller = useAssetFilters({ family, canReadAccounts: allowed, accountApi: { list } })
    return () => h(AssetFilters, { controller, locationType, onChange: change })
  } }), { global: { stubs: { 'a-input': textControl, 'a-select': control, 'a-input-number': control, 'a-checkbox': control } } })
  return { wrapper, controller, list, change }
}
describe('AssetFilters', () => {
  it('offers provider, scope and instance filters and uses numeric account input without permission', async () => {
    const { wrapper, controller, list, change } = setup()
    expect(wrapper.find('[data-testid="account-id"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="account"]').exists()).toBe(false)
    for (const field of ['provider', 'project_id', 'region', 'zone', 'q', 'state', 'vpc_id', 'ip', 'tag_key', 'tag_value']) expect(wrapper.find(`[data-testid="${field}"]`).exists()).toBe(true)
    await wrapper.find('[data-testid="provider"]').setValue('gcp')
    await wrapper.find('[data-testid="region"]').setValue('future-region')
    expect(controller.filters.value).toMatchObject({ provider: 'gcp', region: 'future-region' })
    expect(change).toHaveBeenCalledTimes(2)
    expect(list).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('uses read-authorized account choices and clears scope on account changes', async () => {
    const { wrapper, controller, list } = setup('database', ref(true))
    await flushPromises()
    expect(list).toHaveBeenCalledOnce()
    expect(wrapper.find('[data-testid="account"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="engine"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="status"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="state"]').exists()).toBe(false)
    controller.setFilters({ account_id: 7, region: 'region-a', zone: 'zone-a' })
    await nextTick()
    await wrapper.find('[data-testid="account"]').setValue('8')
    expect(controller.filters.value.region).toBeUndefined()
    wrapper.unmount()
  })
  it('shows global context distinctly without sending a fabricated location filter', async () => {
    const { wrapper, controller } = setup('firewall', ref(false), 'global')
    expect(wrapper.find('[data-testid="location-context"]').text()).toContain('assets.filters.location.global')
    expect(wrapper.find('[data-testid="network_ref"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="vpc_id"]').exists()).toBe(false)
    expect(controller.filters.value).toEqual({})
    wrapper.unmount()
    for (const type of ['region', 'zone'] as const) {
      const next = setup('vpc', ref(false), type)
      expect(next.wrapper.find('[data-testid="location-context"]').text()).toContain(`assets.filters.location.${type}`)
      next.wrapper.unmount()
    }
  })
})


describe('AssetFilters actual Ant Design controls', () => {
  it('preserves intermediate spaces when typing a multi-word query character by character', async () => {
    let controller!: AssetFiltersController
    const wrapper = mount(defineComponent({ setup() {
      controller = useAssetFilters({ family: 'instance', canReadAccounts: false })
      return () => h(AssetFilters, { controller })
    } }), { global: { components: { AInput: Input, ASelect: Select, AInputNumber: InputNumber, ACheckbox: Checkbox } } })
    const input = wrapper.get<HTMLInputElement>('[data-testid="q"] input, input[data-testid="q"]')
    for (const character of 'web server') {
      await input.setValue(input.element.value + character)
      await nextTick()
    }
    expect(input.element.value).toBe('web server')
    expect(controller.filters.value.q).toBe('web server')
    wrapper.unmount()
  })
})
