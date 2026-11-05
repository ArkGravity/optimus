/* eslint-disable vue/one-component-per-file, vue/require-prop-types */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createVNode, defineComponent, h, ref } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { message, RadioButton, RadioGroup } from 'ant-design-vue'
import type * as AntDesignVue from 'ant-design-vue'
import type { CloudKeySummary } from '@/types/api'
import CloudKeyEditModal from './CloudKeyEditModal.vue'
import modalSource from './CloudKeyEditModal.vue?raw'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('ant-design-vue', async (importOriginal) => ({
  ...await importOriginal<typeof AntDesignVue>(),
  message: { success: vi.fn(), error: vi.fn() },
}))

const clearValidate = vi.fn()
const Modal = {
  props: ['open'], emits: ['ok', 'cancel'],
  template: '<div v-if="open"><slot/><button class="submit" @click="$emit(\'ok\')">save</button><button class="cancel" @click="$emit(\'cancel\')">cancel</button></div>',
}
const Form = defineComponent({
  props: ['model', 'rules'],
  setup(props, { slots, expose }) {
    expose({
      validate: async () => {
        for (const [name, rules] of Object.entries(props.rules as Record<string, { required?: boolean }[]>)) {
          if (rules.some(rule => rule.required) && !props.model[name]) throw new Error('required')
        }
      },
      resetFields: vi.fn(), clearValidate,
    })
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
  'a-modal': Modal, 'a-form': Form,
  'a-form-item': { props: ['name'], template: '<div :data-field="name"><slot/></div>' },
  'a-input': Input, 'a-textarea': Input, 'a-input-password': Input,
  'a-alert': { props: ['message'], template: '<div role="alert">{{ message }}</div>' },
}

const wrappers: VueWrapper[] = []
function setup(initial?: Partial<CloudKeySummary>) {
  const api = { create: vi.fn().mockResolvedValue({}), update: vi.fn().mockResolvedValue({}) }
  const wrapper = mount(CloudKeyEditModal, {
    props: {
      open: true,
      initial: initial && {
        id: 7, name: 'existing', description: 'description', region: 'region',
        created_at: '', updated_at: '', provider: 'aws', auth_type: 'access_key',
        service_account_email: '', credential_project_id: '', needs_configuration: false,
        ...initial,
      },
    },
    global: {
      provide: { cloudKeyApi: api }, stubs,
      mocks: { $t: (key: string) => key },
      components: { 'a-radio-group': RadioGroup, 'a-radio-button': RadioButton },
    },
  })
  wrappers.push(wrapper)
  return { api, wrapper }
}

function summary(overrides: Partial<CloudKeySummary> = {}): CloudKeySummary {
  return {
    id: 7, name: 'existing', description: 'description', region: 'region',
    created_at: '', updated_at: '', provider: 'gcp', auth_type: 'service_account_json',
    service_account_email: '', credential_project_id: '', needs_configuration: false,
    ...overrides,
  }
}

function setupParent(record: CloudKeySummary | null = null) {
  const open = ref(false)
  const initial = ref<CloudKeySummary | null>(record)
  const api = { create: vi.fn().mockResolvedValue({}), update: vi.fn().mockResolvedValue({}) }
  const Parent = defineComponent({
    // Match the optimized dynamic-prop patch emitted by the List.vue template.
    setup: () => () => createVNode(CloudKeyEditModal, {
      open: open.value, initial: initial.value,
      'onUpdate:open': (value: boolean) => { open.value = value },
    }, null, 8, ['open', 'initial']),
  })
  const wrapper = mount(Parent, {
    global: {
      provide: { cloudKeyApi: api }, stubs,
      mocks: { $t: (key: string) => key },
      components: { 'a-radio-group': RadioGroup, 'a-radio-button': RadioButton },
    },
  })
  wrappers.push(wrapper)
  return { wrapper, open, initial, api }
}

const input = (wrapper: VueWrapper, name: string) => wrapper.get(`[data-field="${name}"] input:not([type="file"])`)
const selectProvider = (wrapper: VueWrapper, provider: string) => wrapper.get(`input[type="radio"][value="${provider}"]`).setValue()
async function save(wrapper: VueWrapper) {
  await wrapper.get('.submit').trigger('click')
  await flushPromises()
}
async function upload(wrapper: VueWrapper, content: string | Promise<string>, size = 10) {
  const file = new File(['fixture'], 'service-account.json', { type: 'application/json' })
  const text = vi.fn().mockImplementation(() => Promise.resolve(content))
  Object.defineProperties(file, { text: { value: text }, size: { value: size } })
  const fileInput = wrapper.get('input[type="file"]')
  Object.defineProperty(fileInput.element, 'files', { value: [file], configurable: true })
  await fileInput.trigger('change')
  await flushPromises()
  return text
}

describe('Cloud Key provider-specific credentials', () => {
  beforeEach(() => { vi.clearAllMocks() })
  afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

  it('offers Aliyun, AWS and GCP and creates Aliyun with explicit access-key authentication', async () => {
    const { api, wrapper } = setup()
    expect(wrapper.findAll('input[type="radio"]').map(radio => radio.attributes('value')))
      .toEqual(['aliyun', 'aws', 'gcp'])
    await selectProvider(wrapper, 'aliyun')
    await input(wrapper, 'name').setValue('aliyun-production')
    await input(wrapper, 'access_key_id').setValue('aliyun-ak')
    await input(wrapper, 'secret_access_key').setValue('aliyun-sk')
    await save(wrapper)
    expect(api.create).toHaveBeenCalledWith({
      name: 'aliyun-production', description: undefined, provider: 'aliyun', region: undefined,
      auth_type: 'access_key', access_key_id: 'aliyun-ak', secret_access_key: 'aliyun-sk',
    })
    expect(wrapper.emitted('saved')).toBeTruthy()
  })

  it('shows JSON upload and paste for GCP and submits no access-key fields', async () => {
    const { api, wrapper } = setup()
    await selectProvider(wrapper, 'gcp')
    expect(wrapper.find('[data-field="access_key_id"]').exists()).toBe(false)
    expect(wrapper.find('[data-field="secret_access_key"]').exists()).toBe(false)
    expect(wrapper.get('input[type="file"]').attributes('accept')).toContain('.json')
    await input(wrapper, 'name').setValue('gcp')
    await input(wrapper, 'description').setValue('GCP metadata')
    await input(wrapper, 'region').setValue('us-central1')
    await input(wrapper, 'service_account_json').setValue('{"private_key":"paste-fixture"}')
    await save(wrapper)
    expect(api.create).toHaveBeenCalledWith({
      name: 'gcp', description: 'GCP metadata', region: 'us-central1', provider: 'gcp',
      auth_type: 'service_account_json', service_account_json: '{"private_key":"paste-fixture"}',
    })
  })

  it('reads a file up to 64 KiB into the draft without an automatic API request', async () => {
    const { api, wrapper } = setup()
    await selectProvider(wrapper, 'gcp')
    const text = await upload(wrapper, '{"private_key":"file-fixture"}', 64 * 1024)
    expect(text).toHaveBeenCalledOnce()
    expect(input(wrapper, 'service_account_json').element).toHaveProperty('value', '{"private_key":"file-fixture"}')
    expect(api.create).not.toHaveBeenCalled()
    expect(api.update).not.toHaveBeenCalled()
  })

  it('rejects files over 64 KiB before reading their contents', async () => {
    const { wrapper } = setup()
    await selectProvider(wrapper, 'gcp')
    const text = await upload(wrapper, 'secret', 64 * 1024 + 1)
    expect(text).not.toHaveBeenCalled()
    expect(input(wrapper, 'service_account_json').element).toHaveProperty('value', '')
    expect(message.error).toHaveBeenCalledWith('credentials.cloud_keys.json_too_large')
  })

  it('clears every secret and auth validation on provider switches while preserving metadata', async () => {
    const { wrapper } = setup()
    await input(wrapper, 'name').setValue('metadata')
    await input(wrapper, 'access_key_id').setValue('old-ak')
    await input(wrapper, 'secret_access_key').setValue('old-sk')
    await selectProvider(wrapper, 'gcp')
    await input(wrapper, 'service_account_json').setValue('old-json')
    await selectProvider(wrapper, 'aliyun')
    expect(input(wrapper, 'access_key_id').element).toHaveProperty('value', '')
    expect(input(wrapper, 'secret_access_key').element).toHaveProperty('value', '')
    await selectProvider(wrapper, 'gcp')
    expect(input(wrapper, 'service_account_json').element).toHaveProperty('value', '')
    expect(input(wrapper, 'name').element).toHaveProperty('value', 'metadata')
    expect(clearValidate).toHaveBeenCalled()
  })

  it.each(['aws', 'aliyun', 'gcp'] as const)('keeps %s metadata edits free of secret and auth-type fields', async (provider) => {
    const { api, wrapper } = setup({ provider, auth_type: provider === 'gcp' ? 'service_account_json' : 'access_key' })
    await input(wrapper, 'name').setValue('renamed')
    await save(wrapper)
    expect(api.update).toHaveBeenCalledWith(7, { name: 'renamed' })
    expect(api.create).not.toHaveBeenCalled()
  })

  it('shows GCP identity metadata without preloading JSON', () => {
    const { wrapper } = setup({
      provider: 'gcp', auth_type: 'service_account_json',
      service_account_email: 'fixture@example.iam.gserviceaccount.com', credential_project_id: 'fixture-project',
    })
    expect(wrapper.text()).toContain('fixture@example.iam.gserviceaccount.com')
    expect(wrapper.text()).toContain('fixture-project')
    expect(input(wrapper, 'service_account_json').element).toHaveProperty('value', '')
  })

  it('allows a legacy GCP metadata edit without requesting auth conversion', async () => {
    const { api, wrapper } = setup({ provider: 'gcp', auth_type: 'legacy_invalid', needs_configuration: true })
    expect(wrapper.text()).toContain('credentials.cloud_keys.needs_configuration')
    await input(wrapper, 'name').setValue('legacy renamed')
    await save(wrapper)
    expect(api.update).toHaveBeenCalledWith(7, { name: 'legacy renamed' })
  })

  it('repairs a legacy GCP credential with a complete JSON authentication shape', async () => {
    const { api, wrapper } = setup({ provider: 'gcp', auth_type: 'legacy_invalid', needs_configuration: true })
    await input(wrapper, 'service_account_json').setValue('replacement-json')
    await save(wrapper)
    expect(api.update).toHaveBeenCalledWith(7, {
      provider: 'gcp', auth_type: 'service_account_json', service_account_json: 'replacement-json',
    })
  })

  it.each(['aws', 'gcp'] as const)('requires complete secrets when converting from Aliyun to %s', async (provider) => {
    const { api, wrapper } = setup({ provider: 'aliyun' })
    await selectProvider(wrapper, provider)
    if (provider === 'aws') await input(wrapper, 'access_key_id').setValue('only-one-half')
    await save(wrapper)
    expect(api.update).not.toHaveBeenCalled()
    if (provider === 'aws') await input(wrapper, 'secret_access_key').setValue('replacement-sk')
    else await input(wrapper, 'service_account_json').setValue('replacement-json')
    await save(wrapper)
    expect(api.update).toHaveBeenCalledWith(7, provider === 'aws'
      ? { provider: 'aws', auth_type: 'access_key', access_key_id: 'only-one-half', secret_access_key: 'replacement-sk' }
      : { provider: 'gcp', auth_type: 'service_account_json', service_account_json: 'replacement-json' })
  })

  it('rotates same-type credentials with a complete secret shape', async () => {
    const { api, wrapper } = setup({ provider: 'aws' })
    await input(wrapper, 'access_key_id').setValue('rotated-ak')
    await input(wrapper, 'secret_access_key').setValue('rotated-sk')
    await save(wrapper)
    expect(api.update).toHaveBeenCalledWith(7, {
      provider: 'aws', auth_type: 'access_key', access_key_id: 'rotated-ak', secret_access_key: 'rotated-sk',
    })
  })

  it.each((['aws', 'gcp'] as const).flatMap(provider =>
    (['cancel', 'close', 'unmount', 'save'] as const).map(action => ({ provider, action })),
  ))('clears $provider secret drafts on $action', async ({ provider, action }) => {
    const { wrapper } = setup({ provider, auth_type: provider === 'gcp' ? 'service_account_json' : 'access_key' })
    if (provider === 'gcp') await input(wrapper, 'service_account_json').setValue('must-be-cleared')
    else {
      await input(wrapper, 'access_key_id').setValue('must-be-cleared-ak')
      await input(wrapper, 'secret_access_key').setValue('must-be-cleared-sk')
    }
    const model = wrapper.findComponent(Form).props('model')
    if (action === 'cancel') await wrapper.get('.cancel').trigger('click')
    if (action === 'close') await wrapper.setProps({ open: false })
    if (action === 'unmount') wrapper.unmount()
    if (action === 'save') await save(wrapper)
    expect(model.service_account_json).toBe('')
    expect(model.access_key_id).toBe('')
    expect(model.secret_access_key).toBe('')
  })

  it.each(['provider', 'reopen', 'unmount'] as const)('ignores a late file read after %s changes', async (change) => {
    const { wrapper } = setup({ provider: 'gcp', auth_type: 'service_account_json' })
    let finish!: (value: string) => void
    await upload(wrapper, new Promise<string>(resolve => { finish = resolve }))
    const model = wrapper.findComponent(Form).props('model')
    if (change === 'provider') {
      await selectProvider(wrapper, 'aws')
      await selectProvider(wrapper, 'gcp')
    }
    if (change === 'reopen') {
      await wrapper.setProps({ open: false })
      await wrapper.setProps({ open: true })
    }
    if (change === 'unmount') wrapper.unmount()
    finish('stale-secret')
    await flushPromises()
    expect(model.service_account_json).toBe('')
  })

  it('resets an edit before opening a new credential', async () => {
    const { wrapper } = setup({ provider: 'aliyun' })
    await wrapper.setProps({ open: false, initial: null })
    await wrapper.setProps({ open: true })
    expect(wrapper.get('input[value="aws"]').element).toHaveProperty('checked', true)
    expect(input(wrapper, 'name').element).toHaveProperty('value', '')
    expect(input(wrapper, 'access_key_id').element).toHaveProperty('value', '')
  })

  it('initializes the first GCP edit after its parent updates open and initial together', async () => {
    const { wrapper, open, initial, api } = setupParent()
    initial.value = summary({ id: 42, name: 'first-gcp', description: 'first-description', region: 'us-central1' })
    open.value = true
    await flushPromises()
    expect(input(wrapper, 'name').element).toHaveProperty('value', 'first-gcp')
    expect(input(wrapper, 'description').element).toHaveProperty('value', 'first-description')
    expect(input(wrapper, 'region').element).toHaveProperty('value', 'us-central1')
    expect(wrapper.get('input[value="gcp"]').element).toHaveProperty('checked', true)
    expect(input(wrapper, 'service_account_json').element).toHaveProperty('value', '')
    await input(wrapper, 'name').setValue('renamed-first')
    await save(wrapper)
    expect(api.update).toHaveBeenCalledWith(42, { name: 'renamed-first' })
    expect(api.create).not.toHaveBeenCalled()
  })

  it('initializes record B after editing A and keeps B metadata and secrets isolated', async () => {
    const { wrapper, open, initial, api } = setupParent(summary({ id: 7, name: 'record-a' }))
    open.value = true
    await flushPromises()
    expect(input(wrapper, 'name').element).toHaveProperty('value', 'record-a')
    let finish!: (value: string) => void
    await upload(wrapper, new Promise<string>(resolve => { finish = resolve }))
    open.value = false
    await flushPromises()
    initial.value = summary({ id: 8, name: 'record-b', description: 'description-b', region: 'europe-west1' })
    open.value = true
    await flushPromises()
    finish('record-a-secret')
    await flushPromises()
    expect(input(wrapper, 'name').element).toHaveProperty('value', 'record-b')
    expect(input(wrapper, 'description').element).toHaveProperty('value', 'description-b')
    expect(input(wrapper, 'region').element).toHaveProperty('value', 'europe-west1')
    expect(input(wrapper, 'service_account_json').element).toHaveProperty('value', '')
    await input(wrapper, 'description').setValue('edited-b')
    await save(wrapper)
    expect(api.update).toHaveBeenCalledWith(8, { description: 'edited-b' })
    expect(api.create).not.toHaveBeenCalled()
  })

  it('opens a clean create form after a parent-mounted edit', async () => {
    const { wrapper, open, initial, api } = setupParent(summary({ name: 'prior-gcp' }))
    open.value = true
    await flushPromises()
    await input(wrapper, 'service_account_json').setValue('prior-gcp-secret')
    await wrapper.get('.cancel').trigger('click')
    await flushPromises()
    initial.value = null
    open.value = true
    await flushPromises()
    expect(input(wrapper, 'name').element).toHaveProperty('value', '')
    expect(input(wrapper, 'description').element).toHaveProperty('value', '')
    expect(input(wrapper, 'region').element).toHaveProperty('value', '')
    expect(wrapper.get('input[value="aws"]').element).toHaveProperty('checked', true)
    expect(input(wrapper, 'access_key_id').element).toHaveProperty('value', '')
    await input(wrapper, 'name').setValue('new-aws')
    await input(wrapper, 'access_key_id').setValue('new-ak')
    await input(wrapper, 'secret_access_key').setValue('new-sk')
    await save(wrapper)
    expect(api.create).toHaveBeenCalledWith({
      name: 'new-aws', description: undefined, region: undefined,
      provider: 'aws', auth_type: 'access_key', access_key_id: 'new-ak', secret_access_key: 'new-sk',
    })
    expect(api.update).not.toHaveBeenCalled()
  })

  it('keeps secret content out of browser storage and workspace state', async () => {
    const storageWrite = vi.spyOn(Storage.prototype, 'setItem')
    const { wrapper } = setup()
    await selectProvider(wrapper, 'gcp')
    await input(wrapper, 'service_account_json').setValue('memory-only-secret')
    await upload(wrapper, 'memory-only-file')
    expect(storageWrite).not.toHaveBeenCalled()
    expect(modalSource).not.toMatch(/localStorage|sessionStorage|usePageState|fetch\(|XMLHttpRequest/)
    storageWrite.mockRestore()
  })
})
