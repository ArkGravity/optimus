/* eslint-disable vue/require-prop-types */
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { permissionDirective } from '@/directives/permission'
import { useAuthStore } from '@/stores/auth'
import { useDockerStore } from '@/stores/docker'
import Containers from './Containers.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const { success, showError } = vi.hoisted(() => ({ success: vi.fn(), showError: vi.fn() }))
vi.mock('ant-design-vue', () => ({ message: { success, error: showError } }))

const container = (id: string, state: string) => ({
  id, short_id: id.slice(0, 12), name: `app-${id}`, image: 'nginx:1.27', image_id: 'sha256:abc',
  command: 'nginx', state, status: state === 'running' ? 'Up 2 hours' : 'Exited (0)', ports: ['0.0.0.0:8080->80/tcp'], created: '2026-11-18T10:00:00Z',
})
const ATable = defineComponent({
  props: ['columns', 'dataSource'],
  setup(props, { slots }) {
    return () => h('div', (props.dataSource ?? []).map((record: { id: string }) =>
      h('div', { class: `table-row row-${record.id}` }, (props.columns ?? []).map((column: unknown) => slots.bodyCell?.({ column, record })))))
  },
})
const stubs = {
  'a-card': { template: '<div><slot/></div>' },
  'a-space': { template: '<div><slot/></div>' },
  'a-alert': { props: ['message'], template: '<p class="alert">{{ message }}</p>' },
  'a-input': true, 'a-tag': { template: '<span><slot/></span>' },
  'a-button': { template: '<button><slot/></button>' },
  'a-checkbox': { props: ['checked'], emits: ['update:checked'], template: '<input type="checkbox" class="force" :checked="checked" @change="$emit(\'update:checked\', $event.target.checked)" />' },
  'a-popconfirm': {
    emits: ['confirm', 'openChange'],
    template: '<span><slot/><span class="popconfirm-title"><slot name="title"/></span><button class="open" @click="$emit(\'openChange\', true)">open</button><button class="confirm" @click="$emit(\'confirm\')">ok</button></span>',
  },
  'a-table': ATable, 'a-drawer': true, 'a-spin': true, 'a-descriptions': true, 'a-descriptions-item': true,
  'a-collapse': true, 'a-collapse-panel': true, HostPicker: true,
}

function mountContainers(permissions: string[]) {
  useAuthStore().setPermissions(permissions)
  const api = {
    list: vi.fn().mockResolvedValue({ items: [container('aaa', 'running'), container('bbb', 'exited')], total: 2 }),
    get: vi.fn(), action: vi.fn().mockResolvedValue(undefined), remove: vi.fn().mockResolvedValue(undefined),
  }
  const wrapper = mount(Containers, { global: {
    provide: { dockerContainerApi: api }, stubs,
    mocks: { $t: (key: string) => key },
    directives: { permission: permissionDirective },
  } })
  return { wrapper, api }
}

describe('Docker Containers', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    success.mockClear()
    showError.mockClear()
  })

  it('prompts for a host and loads only after an explicit selection', async () => {
    const { wrapper, api } = mountContainers(['docker:environment:read'])
    await flushPromises()
    expect(wrapper.findComponent({ name: 'HostPicker' }).exists()).toBe(true)
    expect(wrapper.text()).toContain('docker.host.no_host_selected')
    expect(api.list).not.toHaveBeenCalled()
    useDockerStore().setHost(3, 'build-01')
    await flushPromises()
    expect(api.list).toHaveBeenCalledWith(3)
    expect(wrapper.text()).not.toContain('docker.host.no_host_selected')
    expect(wrapper.findAll('.table-row')).toHaveLength(2)
    wrapper.unmount()
  })

  it('hides every mutation control without the write permission', async () => {
    useDockerStore().setHost(3, 'build-01')
    const { wrapper } = mountContainers(['docker:environment:read'])
    await flushPromises()
    expect(wrapper.findAll('.table-row')).toHaveLength(2)
    expect(wrapper.find('.container-action').exists()).toBe(false)
    expect(wrapper.find('.delete-container').exists()).toBe(false)
    wrapper.unmount()
  })

  it('enables lifecycle actions by state', async () => {
    useDockerStore().setHost(3, 'build-01')
    const { wrapper, api } = mountContainers(['docker:environment:read', 'docker:environment:write'])
    await flushPromises()
    const running = wrapper.get('.row-aaa')
    const exited = wrapper.get('.row-bbb')
    expect(running.get('.container-start').classes()).toContain('disabled')
    expect(running.get('.container-stop').classes()).not.toContain('disabled')
    expect(exited.get('.container-start').classes()).not.toContain('disabled')
    expect(exited.get('.container-stop').classes()).toContain('disabled')
    await running.get('.container-start').trigger('click')
    expect(api.action).not.toHaveBeenCalled()
    await running.get('.container-stop').trigger('click')
    await flushPromises()
    expect(api.action).toHaveBeenCalledWith(3, 'aaa', 'stop')
    wrapper.unmount()
  })

  it('deletes a container only after confirmation, forcing running containers by default', async () => {
    useDockerStore().setHost(3, 'build-01')
    const { wrapper, api } = mountContainers(['docker:environment:read', 'docker:environment:write'])
    await flushPromises()
    const running = wrapper.get('.row-aaa')
    await running.get('.delete-container').trigger('click')
    expect(api.remove).not.toHaveBeenCalled()
    await running.get('.open').trigger('click')
    expect((running.get('.force').element as HTMLInputElement).checked).toBe(true)
    await running.get('.confirm').trigger('click')
    await flushPromises()
    expect(api.remove).toHaveBeenCalledWith(3, 'aaa', true)

    const exited = wrapper.get('.row-bbb')
    await exited.get('.open').trigger('click')
    expect((exited.get('.force').element as HTMLInputElement).checked).toBe(false)
    await exited.get('.confirm').trigger('click')
    await flushPromises()
    expect(api.remove).toHaveBeenLastCalledWith(3, 'bbb', false)
    wrapper.unmount()
  })
})
