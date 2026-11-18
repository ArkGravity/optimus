/* eslint-disable vue/require-prop-types */
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDockerStore } from '@/stores/docker'
import HostPicker from './HostPicker.vue'

const ASelect = defineComponent({
  name: 'ASelect', props: ['value', 'options'], emits: ['change'],
  setup(props) { return () => h('div', { class: 'select-stub' }, String(props.value ?? '')) },
})
const host = (id: number, name: string) => ({
  id, name, description: '', host: `10.0.0.${id}`, port: 22, socket_path: '/var/run/docker.sock',
  auth_type: 'password' as const, username: 'root', created_at: '', updated_at: '',
})

function mountPicker(list: ReturnType<typeof vi.fn>) {
  return mount(HostPicker, { global: {
    provide: { dockerHostApi: { list } },
    mocks: { $t: (key: string) => key },
    stubs: { 'a-select': ASelect, 'a-space': { template: '<div><slot/></div>' }, 'a-button': { template: '<button><slot/></button>' } },
  } })
}

describe('HostPicker', () => {
  beforeEach(() => { setActivePinia(createPinia()) })

  it('lists hosts without auto-selecting one and stores an explicit choice', async () => {
    const list = vi.fn().mockResolvedValue({ items: [host(1, 'alpha'), host(2, 'beta')], total: 2 })
    const wrapper = mountPicker(list)
    await flushPromises()
    const docker = useDockerStore()
    const select = wrapper.findComponent(ASelect)
    expect(select.props('options')).toEqual([{ value: 1, label: 'alpha' }, { value: 2, label: 'beta' }])
    expect(docker.selectedHostId).toBeNull()
    expect(select.props('value')).toBeUndefined()
    select.vm.$emit('change', 2)
    expect(docker.selectedHostId).toBe(2)
    expect(docker.selectedHostName).toBe('beta')
    select.vm.$emit('change', undefined)
    expect(docker.selectedHostId).toBeNull()
    wrapper.unmount()
  })

  it('does not auto-select when exactly one host exists', async () => {
    const wrapper = mountPicker(vi.fn().mockResolvedValue({ items: [host(5, 'only')], total: 1 }))
    await flushPromises()
    expect(useDockerStore().selectedHostId).toBeNull()
    wrapper.unmount()
  })

  it('refreshes a renamed selection and clears a deleted one instead of choosing another', async () => {
    const docker = useDockerStore()
    docker.setHost(1, 'old-name')
    const renamed = mountPicker(vi.fn().mockResolvedValue({ items: [host(1, 'new-name')], total: 1 }))
    await flushPromises()
    expect(docker.selectedHostName).toBe('new-name')
    renamed.unmount()

    const deleted = mountPicker(vi.fn().mockResolvedValue({ items: [host(2, 'other')], total: 1 }))
    await flushPromises()
    expect(docker.selectedHostId).toBeNull()
    deleted.unmount()
  })
})
