/* eslint-disable vue/require-prop-types */
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { permissionDirective } from '@/directives/permission'
import { useAuthStore } from '@/stores/auth'
import { useDockerStore } from '@/stores/docker'
import Images from './Images.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const { success, showError } = vi.hoisted(() => ({ success: vi.fn(), showError: vi.fn() }))
vi.mock('ant-design-vue', () => ({ message: { success, error: showError } }))

const ATable = defineComponent({
  props: ['columns', 'dataSource'],
  setup(props, { slots }) {
    return () => h('div', (props.dataSource ?? []).map((record: unknown) =>
      h('div', { class: 'table-row' }, (props.columns ?? []).map((column: unknown) => slots.bodyCell?.({ column, record })))))
  },
})
const stubs = {
  'a-card': { template: '<div><slot/></div>' },
  'a-space': { template: '<div><slot/></div>' },
  'a-alert': { props: ['message'], template: '<p>{{ message }}</p>' },
  'a-input': true, 'a-popconfirm': { template: '<span><slot/></span>' },
  'a-button': { props: ['disabled'], emits: ['click'], template: '<button :disabled="disabled" @click="$emit(\'click\')"><slot/></button>' },
  'a-checkbox': { props: ['checked'], emits: ['update:checked'], template: '<input type="checkbox" class="prune-all" :checked="checked" @change="$emit(\'update:checked\', $event.target.checked)" />' },
  'a-modal': { props: ['open'], emits: ['ok'], template: '<div v-if="open" class="modal"><slot/><button class="modal-ok" @click="$emit(\'ok\')">ok</button></div>' },
  'a-table': ATable, HostPicker: true,
}

function mountImages(permissions: string[]) {
  useAuthStore().setPermissions(permissions)
  const api = {
    list: vi.fn().mockResolvedValue({ items: [{ id: 'sha256:1', short_id: '1', repo_tags: [], dangling: true, size: 1500, created: '' }], total: 1 }),
    remove: vi.fn(),
    prune: vi.fn().mockResolvedValue({ untagged: [], deleted: ['sha256:1'], space_reclaimed: 1500 }),
  }
  const wrapper = mount(Images, { global: {
    provide: { dockerImageApi: api }, stubs,
    mocks: { $t: (key: string) => key },
    directives: { permission: permissionDirective },
  } })
  return { wrapper, api }
}

describe('Docker Images', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    success.mockClear()
  })

  it('requires a host and hides prune/delete for read-only users', async () => {
    const { wrapper, api } = mountImages(['docker:environment:read'])
    await flushPromises()
    expect(wrapper.text()).toContain('docker.host.no_host_selected')
    expect(api.list).not.toHaveBeenCalled()
    useDockerStore().setHost(2, 'edge')
    await flushPromises()
    expect(wrapper.text()).toContain('<none>:<none>')
    expect(wrapper.find('.prune-images').exists()).toBe(false)
    expect(wrapper.find('.delete-image').exists()).toBe(false)
    wrapper.unmount()
  })

  it('prunes only after confirming in the dialog', async () => {
    useDockerStore().setHost(2, 'edge')
    const { wrapper, api } = mountImages(['docker:environment:read', 'docker:environment:write'])
    await flushPromises()
    await wrapper.get('.prune-images').trigger('click')
    expect(api.prune).not.toHaveBeenCalled()
    await wrapper.get('.prune-all').setValue(true)
    await wrapper.get('.modal-ok').trigger('click')
    await flushPromises()
    expect(api.prune).toHaveBeenCalledWith(2, true)
    expect(wrapper.find('.modal').exists()).toBe(false)
    wrapper.unmount()
  })
})
