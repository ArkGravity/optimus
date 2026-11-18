/* eslint-disable vue/require-prop-types */
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { permissionDirective } from '@/directives/permission'
import { useAuthStore } from '@/stores/auth'
import { useDockerStore } from '@/stores/docker'
import Hosts from './Hosts.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const { success, showError } = vi.hoisted(() => ({ success: vi.fn(), showError: vi.fn() }))
vi.mock('ant-design-vue', () => ({ message: { success, error: showError } }))

const row = {
  id: 4, name: 'build-01', description: '', host: '10.0.0.4', port: 2222, socket_path: '/var/run/docker.sock',
  auth_type: 'ssh_key' as const, ssh_key_id: 9, ssh_key_name: 'deploy-key', username: 'deploy',
  last_check_ok: false, last_check_error: 'unreachable', created_at: '2026-11-18T10:00:00Z', updated_at: '2026-11-18T10:00:00Z',
}
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
  'a-input-search': true, 'a-pagination': true, 'a-tooltip': { template: '<span><slot/></span>' },
  'a-badge': { props: ['text'], template: '<span>{{ text }}</span>' },
  'a-button': { template: '<button><slot/></button>' },
  'a-popconfirm': { emits: ['confirm'], template: '<span><slot/><button class="confirm" @click="$emit(\'confirm\')">ok</button></span>' },
  'a-table': ATable,
  HostForm: true,
}

function mountHosts(permissions: string[]) {
  useAuthStore().setPermissions(permissions)
  const api = {
    list: vi.fn().mockResolvedValue({ items: [row], total: 1, page: 1, page_size: 20 }),
    get: vi.fn(), remove: vi.fn().mockResolvedValue(undefined),
    test: vi.fn().mockResolvedValue({ ok: true, server_version: '27.3.1', checked_at: '' }),
  }
  const wrapper = mount(Hosts, { global: {
    provide: { dockerHostApi: api }, stubs,
    mocks: { $t: (key: string) => key },
    directives: { permission: permissionDirective },
  } })
  return { wrapper, api }
}

describe('Docker Hosts', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    success.mockClear()
    showError.mockClear()
  })

  it('renders host columns and hides mutation controls for read-only users', async () => {
    const { wrapper } = mountHosts(['docker:environment:read'])
    await flushPromises()
    expect(wrapper.text()).toContain('10.0.0.4:2222')
    expect(wrapper.text()).toContain('deploy-key')
    expect(wrapper.text()).toContain('docker.host.unreachable')
    expect(wrapper.find('.create-host').exists()).toBe(false)
    expect(wrapper.find('.edit-host').exists()).toBe(false)
    expect(wrapper.find('.delete-host').exists()).toBe(false)
    expect(wrapper.find('.test-host').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows mutation controls with write and clears the picker selection on delete', async () => {
    const { wrapper, api } = mountHosts(['docker:environment:read', 'docker:environment:write'])
    await flushPromises()
    expect(wrapper.find('.create-host').exists()).toBe(true)
    expect(wrapper.find('.edit-host').exists()).toBe(true)
    expect(wrapper.find('.delete-host').exists()).toBe(true)
    const docker = useDockerStore()
    docker.setHost(4, 'build-01')
    await wrapper.get('.confirm').trigger('click')
    await flushPromises()
    expect(api.remove).toHaveBeenCalledWith(4)
    expect(docker.selectedHostId).toBeNull()
    wrapper.unmount()
  })
})
