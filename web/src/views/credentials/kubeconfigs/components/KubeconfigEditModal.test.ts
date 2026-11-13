/* eslint-disable vue/one-component-per-file, vue/require-prop-types */
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createVNode, defineComponent, h, ref } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { KubeconfigSummary } from '@/types/api'
import KubeconfigEditModal from './KubeconfigEditModal.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('ant-design-vue', () => ({ message: { success: vi.fn(), error: vi.fn() } }))

const Form = defineComponent({
  props: ['model', 'rules'],
  setup(props, { slots, expose }) {
    expose({ resetFields: vi.fn(), clearValidate: vi.fn(), validate: async () => {
      for (const [field, rules] of Object.entries(props.rules as Record<string, { required?: boolean }[]>)) {
        if (rules.some(rule => rule.required) && !props.model[field]) throw new Error('required')
      }
    } })
    return () => h('form', slots.default?.())
  },
})
const Input = defineComponent({
  props: ['value', 'disabled'], emits: ['update:value'],
  setup(props, { emit }) {
    return () => h('input', { value: props.value, disabled: props.disabled, onInput: (event: Event) => emit('update:value', (event.target as HTMLInputElement).value) })
  },
})
const Select = defineComponent({
  props: ['value', 'options', 'disabled'], emits: ['update:value'],
  setup(props, { emit }) {
    return () => h('select', { value: props.value, disabled: props.disabled, onChange: (event: Event) => {
      const value = (event.target as HTMLSelectElement).value
      emit('update:value', props.options.find((option: { value: string | number }) => String(option.value) === value)?.value)
    } }, props.options.map((option: { value: string | number; label: string }) => h('option', { value: option.value }, option.label)))
  },
})
const stubs = {
  'a-modal': { props: ['open'], emits: ['ok', 'cancel'], template: '<div v-if="open"><slot/><button class="save" @click="$emit(\'ok\')">save</button><button class="cancel" @click="$emit(\'cancel\')">cancel</button></div>' },
  'a-form': Form,
  'a-form-item': { props: ['name'], template: '<div :data-field="name"><slot/></div>' },
  'a-input': Input, 'a-textarea': Input, 'a-select': Select,
  'a-alert': { template: '<div/>' },
}
const wrappers: VueWrapper[] = []
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

function record(id = 7): KubeconfigSummary {
  return { id, name: `GKE ${id}`, description: '', default_namespace: '', auth_type: 'gke_service_account', cloud_key_id: 11,
    gke: { project: 'fixture-project', location: 'us-central1', cluster: 'fixture', endpoint: 'private' }, created_at: '', updated_at: '' }
}

function setup(initial?: KubeconfigSummary, parent = false) {
  const api = { create: vi.fn().mockResolvedValue({}), update: vi.fn().mockResolvedValue({}) }
  const cloud = { list: vi.fn().mockResolvedValue({ items: [{ id: 11, name: 'GCP fixture', auth_type: 'service_account_json' }, { id: 12, name: 'legacy', auth_type: 'legacy_invalid' }], total: 2 }) }
  const open = ref(!parent)
  const selected = ref<KubeconfigSummary | null>(initial ?? null)
  const Parent = defineComponent({ setup: () => () => createVNode(KubeconfigEditModal, { open: open.value, initial: selected.value,
    'onUpdate:open': (value: boolean) => { open.value = value } }, null, 8, ['open', 'initial']) })
  const wrapper = mount(parent ? Parent : KubeconfigEditModal, {
    props: parent ? {} : { open: true, initial }, global: { provide: { kubeconfigApi: api, cloudKeyApi: cloud }, stubs, mocks: { $t: (key: string) => key } },
  })
  wrappers.push(wrapper)
  return { wrapper, api, cloud, open, selected }
}
const input = (wrapper: VueWrapper, field: string) => wrapper.get(`[data-field="${field}"] input`)
const select = (wrapper: VueWrapper, field: string) => wrapper.get(`[data-field="${field}"] select`)
async function save(wrapper: VueWrapper) { await wrapper.get('.save').trigger('click'); await flushPromises() }

describe('native GKE Kubernetes credentials', () => {
  it('creates a GKE binding without uploading kubeconfig or copying a token', async () => {
    const { wrapper, api, cloud } = setup()
    await input(wrapper, 'name').setValue('GKE connection')
    await input(wrapper, 'kubeconfig').setValue('old-sensitive-draft')
    await select(wrapper, 'auth_type').setValue('gke_service_account')
    await flushPromises()
    expect(cloud.list).toHaveBeenCalledWith({ page: 1, page_size: 100, provider: 'gcp' })
    expect(wrapper.find('[data-field="kubeconfig"]').exists()).toBe(false)
    expect(select(wrapper, 'cloud_key_id').findAll('option').map(option => option.text())).toEqual(['GCP fixture'])
    await select(wrapper, 'cloud_key_id').setValue('11')
    await input(wrapper, 'project').setValue('fixture-project')
    await input(wrapper, 'location').setValue('us-central1')
    await input(wrapper, 'cluster').setValue('fixture')
    await save(wrapper)
    expect(api.create).toHaveBeenCalledWith({ name: 'GKE connection', description: undefined, default_namespace: undefined,
      auth_type: 'gke_service_account', cloud_key_id: 11, gke: { project: 'fixture-project', location: 'us-central1', cluster: 'fixture', endpoint: 'default' } })
    expect(wrapper.findComponent(Form).props('model').kubeconfig).toBe('')
  })

  it('keeps uploaded kubeconfig behavior and never loads cloud credentials for it', async () => {
    const { wrapper, api, cloud } = setup()
    await input(wrapper, 'name').setValue('standard')
    await save(wrapper)
    expect(api.create).not.toHaveBeenCalled()
    await input(wrapper, 'kubeconfig').setValue('kubeconfig-fixture')
    await save(wrapper)
    expect(api.create).toHaveBeenCalledWith({ name: 'standard', description: undefined, default_namespace: undefined, kubeconfig: 'kubeconfig-fixture' })
    expect(cloud.list).not.toHaveBeenCalled()
  })

  it('preserves immutable GKE targets and performs metadata-only edits', async () => {
    const { wrapper, api } = setup(record())
    await flushPromises()
    expect(select(wrapper, 'auth_type').attributes('disabled')).toBeDefined()
    for (const field of ['project', 'location', 'cluster']) expect(input(wrapper, field).attributes('disabled')).toBeDefined()
    await input(wrapper, 'description').setValue('edited metadata')
    await save(wrapper)
    expect(api.update).toHaveBeenCalledWith(7, { description: 'edited metadata' })
    expect(api.create).not.toHaveBeenCalled()
  })

  it('initializes the intended GKE record when a parent patches open before initial', async () => {
    const { wrapper, open, selected, api } = setup(undefined, true)
    selected.value = record(42)
    open.value = true
    await flushPromises()
    expect(input(wrapper, 'name').element).toHaveProperty('value', 'GKE 42')
    await input(wrapper, 'description').setValue('record 42')
    await save(wrapper)
    expect(api.update).toHaveBeenCalledWith(42, { description: 'record 42' })
  })

  it('blocks incomplete GKE bindings and clears secret drafts on cancel', async () => {
    const { wrapper, api } = setup()
    await input(wrapper, 'name').setValue('incomplete')
    await input(wrapper, 'kubeconfig').setValue('must-clear')
    await wrapper.get('.cancel').trigger('click')
    expect(wrapper.findComponent(Form).props('model').kubeconfig).toBe('')
    await select(wrapper, 'auth_type').setValue('gke_service_account')
    await flushPromises()
    await save(wrapper)
    expect(api.create).not.toHaveBeenCalled()
  })

  it('discards an old save result after reopening a different credential', async () => {
    const { wrapper, api } = setup(record())
    await flushPromises()
    let finish!: () => void
    api.update.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve }))
    await input(wrapper, 'description').setValue('old request')
    await wrapper.get('.save').trigger('click')
    await flushPromises()
    await wrapper.setProps({ open: false })
    await wrapper.setProps({ open: true, initial: record(8) })
    finish()
    await flushPromises()
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(input(wrapper, 'name').element).toHaveProperty('value', 'GKE 8')
    await input(wrapper, 'description').setValue('current request')
    await save(wrapper)
    expect(api.update).toHaveBeenLastCalledWith(8, { description: 'current request' })
  })
})
