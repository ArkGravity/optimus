/* eslint-disable vue/one-component-per-file, vue/require-prop-types */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { createVNode, defineComponent, h, ref } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { Input, Select, Switch, Button, message } from 'ant-design-vue'
import type * as AntDesignVue from 'ant-design-vue'
import { useAuthStore } from '@/stores/auth'
import type { CloudAccountDetail } from '@/types/assets'
import { choose, pending } from '../../components/__tests__/resource-list-contract'
import Form from '../Form.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('ant-design-vue', async (original) => ({ ...await original<typeof AntDesignVue>(), message: { success: vi.fn(), error: vi.fn() } }))
const validate = vi.fn().mockResolvedValue(undefined)
const FormStub = defineComponent({
  props: ['model', 'rules'], setup(_, { slots, expose }) {
    expose({ validate, clearValidate: vi.fn(), resetFields: vi.fn() })
    return () => h('form', slots.default?.())
  },
})
const stubs = {
  'a-modal': { props: ['open', 'confirmLoading'], emits: ['ok', 'cancel'], template: '<div v-if="open"><slot/><button class="save" @click="$emit(\'ok\')">save</button><button class="cancel" @click="$emit(\'cancel\')">cancel</button></div>' },
  'a-form': FormStub, 'a-form-item': { props: ['name'], template: '<div :data-field="name"><slot/></div>' },
  'a-alert': { props: ['message'], template: '<p role="alert">{{ message }}</p>' },
}
const keys = [
  { id: 11, name: 'aws-prod', provider: 'aws', auth_type: 'access_key', needs_configuration: false },
  { id: 12, name: 'aliyun-prod', provider: 'aliyun', auth_type: 'access_key', needs_configuration: false },
  { id: 13, name: 'gcp-prod', provider: 'gcp', auth_type: 'service_account_json', needs_configuration: false, credential_project_id: 'home-project' },
  { id: 14, name: 'legacy-gcp', provider: 'gcp', auth_type: 'legacy_invalid', needs_configuration: true },
]
function account(overrides: Partial<CloudAccountDetail> = {}): CloudAccountDetail {
  return { id: 7, name: 'old', provider: 'gcp', cloudkey_id: 13, cloudkey_name: 'gcp-prod', project_id: 'target-project', external_account_id: 'target-project', scope_revision: 1, regions_count: 1, enabled_regions: ['us-central1'], enabled: false, description: 'old desc', created_at: '', updated_at: '', ...overrides }
}
const mounted: VueWrapper[] = []
function setup(editing: CloudAccountDetail | null = null, permissions = ['credentials:cloud_key:read']) {
  useAuthStore().setPermissions(permissions)
  const api = { create: vi.fn().mockResolvedValue({}), update: vi.fn().mockResolvedValue({}), regionOptions: vi.fn().mockResolvedValue({ items: [{ id: 'region-one', name: 'Region One' }] }) }
  const cloudKey = { list: vi.fn().mockResolvedValue({ items: keys, total: keys.length }), get: vi.fn() }
  const open = ref(true); const record = ref(editing)
  const Parent = defineComponent({ setup: () => () => createVNode(Form, { open: open.value, editing: record.value, onClose: () => { open.value = false } }, null, 8, ['open', 'editing']) })
  const wrapper = mount(Parent, { attachTo: document.body, global: { provide: { assetsAccountApi: api, cloudKeyApi: cloudKey }, stubs, components: { AInput: Input, ATextarea: Input.TextArea, ASelect: Select, ASwitch: Switch, AButton: Button } } })
  mounted.push(wrapper)
  return { wrapper, api, cloudKey, open, record, form: () => wrapper.getComponent(Form) }
}
const input = (wrapper: VueWrapper, field: string) => wrapper.get(`[data-field="${field}"] input`)
async function save(wrapper: VueWrapper) { await wrapper.get('.save').trigger('click'); await flushPromises() }
async function load(wrapper: VueWrapper) { await wrapper.get('[data-testid="load-regions"]').trigger('click'); await flushPromises() }

describe('CloudAccount form provider scope and lifecycle', () => {
  beforeEach(() => { setActivePinia(createPinia()); vi.clearAllMocks(); validate.mockResolvedValue(undefined); vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addListener: vi.fn(), removeListener: vi.fn() }))) })
  afterEach(() => { mounted.splice(0).forEach(w => w.unmount()); document.body.innerHTML = ''; vi.unstubAllGlobals() })
  it.each(['aws', 'aliyun'] as const)('derives %s from a usable key and explicitly selects authenticated regions', async provider => {
    const { wrapper, api, cloudKey } = setup(); await flushPromises()
    expect(cloudKey.list).toHaveBeenCalledWith({ page: 1, page_size: 100 })
    await input(wrapper, 'name').setValue('production'); await choose(wrapper, 'cloudkey', `${provider}-prod`)
    expect(wrapper.get('[data-testid="provider"]').element).toHaveProperty('value', provider === 'aws' ? 'AWS' : 'Aliyun')
    expect(api.regionOptions).not.toHaveBeenCalled(); await load(wrapper)
    expect(api.regionOptions).toHaveBeenCalledWith({ provider, cloudkey_id: provider === 'aws' ? 11 : 12 })
    expect(wrapper.getComponent(FormStub).props('model').enabled_regions).toEqual([])
    await choose(wrapper, 'regions', 'Region One'); await save(wrapper)
    expect(api.create).toHaveBeenCalledWith({ name: 'production', provider, cloudkey_id: provider === 'aws' ? 11 : 12, enabled_regions: ['region-one'], enabled: true, description: '' })
  })
  it('requires explicit GCP project and loads only on request', async () => {
    const { wrapper, api } = setup(); await flushPromises(); await choose(wrapper, 'cloudkey', 'gcp-prod')
    expect(wrapper.text()).toContain('home-project'); expect(input(wrapper, 'project_id').element).toHaveProperty('value', '')
    await save(wrapper); expect(api.create).not.toHaveBeenCalled()
    for (const text of ['t', 'target', 'target-project']) await input(wrapper, 'project_id').setValue(text)
    expect(api.regionOptions).not.toHaveBeenCalled(); await load(wrapper)
    expect(api.regionOptions).toHaveBeenCalledWith({ provider: 'gcp', cloudkey_id: 13, project_id: 'target-project' })
    await choose(wrapper, 'regions', 'Region One'); await input(wrapper, 'name').setValue('gcp'); await save(wrapper)
    expect(api.create).toHaveBeenCalledWith(expect.objectContaining({ provider: 'gcp', project_id: 'target-project' }))
  })
  it('edits immutable binding without credential reads and survives catalog failure', async () => {
    const { wrapper, api, cloudKey } = setup(account(), ['assets:account:write']); await flushPromises()
    expect(cloudKey.list).not.toHaveBeenCalled(); expect(cloudKey.get).not.toHaveBeenCalled()
    expect(wrapper.getComponent(Select).props('disabled')).toBe(true); expect(input(wrapper, 'project_id').attributes('disabled')).toBeDefined()
    api.regionOptions.mockRejectedValueOnce(new Error('private raw failure')); await load(wrapper)
    expect(wrapper.text()).toContain('network.error'); expect(wrapper.text()).not.toContain('private raw failure')
    await input(wrapper, 'name').setValue('renamed'); await save(wrapper)
    expect(api.update).toHaveBeenCalledWith(7, { name: 'renamed', enabled_regions: ['us-central1'], enabled: false, description: 'old desc' })
  })
  it('shows clear create permission state and excludes unusable credentials', async () => {
    const denied = setup(null, []); await flushPromises()
    expect(denied.cloudKey.list).not.toHaveBeenCalled(); expect(denied.wrapper.text()).toContain('assets.account.credential_read_required')
    const allowed = setup(); await flushPromises(); expect(allowed.wrapper.getComponent(Select).props('options')).toHaveLength(3)
  })
  it('pages through all credential summaries', async () => {
    const { wrapper, cloudKey, open } = setup(); await flushPromises(); open.value = false; await flushPromises()
    cloudKey.list.mockResolvedValueOnce({ items: Array.from({ length: 100 }, (_, i) => ({ ...keys[0]!, id: i + 100 })), total: 101 }).mockResolvedValueOnce({ items: [keys[2]!], total: 101 })
    open.value = true; await flushPromises()
    expect(cloudKey.list).toHaveBeenLastCalledWith({ page: 2, page_size: 100 }); expect(wrapper.getComponent(Select).props('options')).toHaveLength(101)
  })
  it('rejects stale catalogs after project or key change and preserves loading ownership', async () => {
    const { wrapper, api } = setup(); await flushPromises(); await choose(wrapper, 'cloudkey', 'gcp-prod'); await input(wrapper, 'project_id').setValue('old-project')
    const old = pending<{ items: { id: string; name: string }[] }>(); const current = pending<{ items: { id: string; name: string }[] }>()
    api.regionOptions.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    await load(wrapper); await input(wrapper, 'project_id').setValue('new-project'); await load(wrapper)
    old.reject(new Error('stale')); await flushPromises(); expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="load-regions"]').attributes('disabled')).toBeDefined()
    current.resolve({ items: [{ id: 'new', name: 'New Region' }] }); await flushPromises(); await choose(wrapper, 'regions', 'New Region'); await choose(wrapper, 'cloudkey', 'aws-prod')
    expect(wrapper.getComponent(FormStub).props('model').enabled_regions).toEqual([]); expect(wrapper.find('[data-field="project_id"]').exists()).toBe(false)
  })
  it('isolates late validation and save feedback from a reopened record', async () => {
    const { wrapper, api, open, record, form } = setup(account()); await flushPromises()
    const validation = pending<void>(); validate.mockReturnValueOnce(validation.promise); await save(wrapper)
    open.value = false; await flushPromises(); record.value = account({ id: 8, name: 'record-b' }); open.value = true; await flushPromises()
    validation.resolve(); await flushPromises(); expect(api.update).not.toHaveBeenCalled()
    const request = pending<object>(); api.update.mockReturnValueOnce(request.promise); await save(wrapper); expect(api.update).toHaveBeenCalledWith(8, expect.anything())
    open.value = false; await flushPromises(); record.value = account({ id: 9, name: 'record-c' }); open.value = true; await flushPromises(); request.resolve({}); await flushPromises()
    expect(form().emitted('saved')).toBeUndefined(); expect(message.success).not.toHaveBeenCalled(); expect(input(wrapper, 'name').element).toHaveProperty('value', 'record-c')
  })
  it('clears drafts on close and ignores late key responses after reopening', async () => {
    const { wrapper, cloudKey, open } = setup(); await flushPromises(); await input(wrapper, 'name').setValue('draft'); const model = wrapper.getComponent(FormStub).props('model')
    open.value = false; await flushPromises(); expect(model.name).toBe('')
    const old = pending<{ items: typeof keys; total: number }>(); cloudKey.list.mockReturnValueOnce(old.promise)
    open.value = true; await flushPromises(); open.value = false; await flushPromises(); open.value = true; await flushPromises(); old.resolve({ items: [], total: 0 }); await flushPromises()
    expect(wrapper.getComponent(Select).props('options')).toHaveLength(3)
  })
  it('locks before validation and preserves validation failure behavior', async () => {
    const { wrapper, api } = setup(account()); await flushPromises(); const validation = pending<void>(); validate.mockReturnValueOnce(validation.promise)
    await save(wrapper); await save(wrapper); expect(validate).toHaveBeenCalledTimes(1); validation.reject(new Error('invalid')); await flushPromises(); expect(api.update).not.toHaveBeenCalled()
  })
  it.each(['key', 'project'] as const)('invalidates pending validation when the %s selection changes in the same modal', async selection => {
    const { wrapper, api } = setup(); await flushPromises()
    await choose(wrapper, 'cloudkey', 'gcp-prod'); await input(wrapper, 'name').setValue('gcp')
    await input(wrapper, 'project_id').setValue('old-project'); await load(wrapper); await choose(wrapper, 'regions', 'Region One')
    const validation = pending<void>(); validate.mockReturnValueOnce(validation.promise); await save(wrapper)
    if (selection === 'project') await input(wrapper, 'project_id').setValue('new-project')
    else await choose(wrapper, 'cloudkey', 'aliyun-prod')
    await load(wrapper); await choose(wrapper, 'regions', 'Region One')
    validation.resolve(); await flushPromises()
    expect(api.create).not.toHaveBeenCalled()
    expect(wrapper.getComponent(FormStub).props('model').enabled_regions).toEqual(['region-one'])
    await save(wrapper)
    expect(api.create).toHaveBeenCalledWith(expect.objectContaining(selection === 'project'
      ? { provider: 'gcp', cloudkey_id: 13, project_id: 'new-project' }
      : { provider: 'aliyun', cloudkey_id: 12 }))
  })
  it('REVIEW preserves changed GCP target while an old create is pending', async () => {
    const { wrapper, api, form } = setup(); await flushPromises()
    await choose(wrapper, 'cloudkey', 'gcp-prod'); await input(wrapper, 'project_id').setValue('old-project')
    await input(wrapper, 'name').setValue('gcp'); await load(wrapper); await choose(wrapper, 'regions', 'Region One')
    const request = pending<object>(); api.create.mockReturnValueOnce(request.promise)
    await save(wrapper); expect(api.create).toHaveBeenCalledWith(expect.objectContaining({ project_id: 'old-project' }))
    await input(wrapper, 'project_id').setValue('new-project')
    expect(input(wrapper, 'project_id').attributes('disabled')).toBeUndefined()
    request.resolve({}); await flushPromises()
    expect(form().emitted('saved')).toBeUndefined()
    expect(input(wrapper, 'project_id').element).toHaveProperty('value', 'new-project')
    expect(message.success).not.toHaveBeenCalled()
  })
})
