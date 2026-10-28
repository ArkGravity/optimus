/* eslint-disable vue/one-component-per-file, vue/require-prop-types */
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { RadioButton, RadioGroup } from 'ant-design-vue'
import type * as AntDesignVue from 'ant-design-vue'
import CloudKeyEditModal from './CloudKeyEditModal.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('ant-design-vue', async (importOriginal) => ({
  ...await importOriginal<typeof AntDesignVue>(),
  message: { success: vi.fn(), error: vi.fn() },
}))

const validate = vi.fn().mockResolvedValue(undefined)
const Modal = {
  props: ['open'], emits: ['ok', 'cancel'],
  template: '<div v-if="open"><slot/><button class="submit" @click="$emit(\'ok\')">save</button></div>',
}
const Form = defineComponent({
  props: ['model', 'rules'],
  setup(_, { slots, expose }) {
    expose({ validate, resetFields: vi.fn() })
    return () => h('form', slots.default?.())
  },
})
const Input = defineComponent({
  props: ['value', 'disabled'], emits: ['update:value'],
  setup(props, { emit }) {
    return () => h('input', {
      value: props.value, disabled: props.disabled,
      onInput: (event: Event) => emit('update:value', (event.target as HTMLInputElement).value),
    })
  },
})
const stubs = {
  'a-modal': Modal, 'a-form': Form, 'a-form-item': { template: '<div><slot/></div>' },
  'a-input': Input, 'a-textarea': Input, 'a-input-password': Input,
}

function setup(initial?: { provider: 'azure' }) {
  const api = { create: vi.fn().mockResolvedValue({}), update: vi.fn().mockResolvedValue({}) }
  const wrapper = mount(CloudKeyEditModal, {
    props: {
      open: true,
      initial: initial && {
        id: 7, name: 'legacy', description: '', region: '',
        created_at: '', updated_at: '', provider: initial.provider,
      },
    },
    global: {
      provide: { cloudKeyApi: api }, stubs,
      mocks: { $t: (key: string) => key },
      components: { 'a-radio-group': RadioGroup, 'a-radio-button': RadioButton },
    },
  })
  return { api, wrapper }
}

describe('Cloud Key provider selection', () => {
  beforeEach(() => { validate.mockClear() })

  it('offers Aliyun, AWS and GCP and submits the selected Aliyun provider', async () => {
    const { api, wrapper } = setup()
    expect(wrapper.findAll('input[type="radio"]').map(input => input.attributes('value')))
      .toEqual(['aliyun', 'aws', 'gcp'])
    const inputs = wrapper.findAll('input:not([type="radio"])')
    await inputs[0]!.setValue('aliyun-production')
    await inputs[3]!.setValue('aliyun-ak')
    await inputs[4]!.setValue('aliyun-sk')
    await wrapper.find('input[value="aliyun"]').setValue()
    await wrapper.find('.submit').trigger('click')
    await flushPromises()
    expect(api.create).toHaveBeenCalledWith({
      name: 'aliyun-production', description: undefined, provider: 'aliyun', region: undefined,
      access_key_id: 'aliyun-ak', secret_access_key: 'aliyun-sk',
    })
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('allows legacy Azure metadata edits without relabeling the credential', async () => {
    const { api, wrapper } = setup({ provider: 'azure' })
    const legacyProvider = wrapper.find('input[disabled]')
    expect((legacyProvider.element as HTMLInputElement).value).toBe('Azure')
    expect(wrapper.findAll('input[type="radio"]')).toHaveLength(0)
    await wrapper.find('input').setValue('renamed')
    await wrapper.find('.submit').trigger('click')
    await flushPromises()
    expect(api.update).toHaveBeenCalledWith(7, { name: 'renamed' })
    expect(api.create).not.toHaveBeenCalled()
  })

  it('resets a legacy edit before opening a new credential', async () => {
    const { api, wrapper } = setup({ provider: 'azure' })
    await wrapper.setProps({ open: false, initial: null })
    await wrapper.setProps({ open: true })
    expect(wrapper.find('input[value="aws"]').element).toHaveProperty('checked', true)
    expect(wrapper.find('input[disabled]').exists()).toBe(false)
    await wrapper.find('.submit').trigger('click')
    await flushPromises()
    expect(api.create).toHaveBeenCalledWith(expect.objectContaining({ provider: 'aws' }))
  })
})
