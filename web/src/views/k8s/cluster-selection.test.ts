import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { useK8sStore } from '@/stores/k8s'
import Workloads from './workloads/List.vue'
import Network from './network/List.vue'
import Config from './config/List.vue'
import ClusterResources from './cluster-resources/List.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe.each([
  ['workloads', Workloads, 'workloadApi'],
  ['network', Network, 'k8sNetworkApi'],
  ['config', Config, 'configMapApi'],
  ['cluster resources', ClusterResources, 'k8sNsApi'],
] as const)('%s cluster selection', (_name, component, apiName) => {
  it('stays on the page without a cluster and loads after an explicit selection', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const list = vi.fn().mockResolvedValue({ items: [], truncated: false })
    const namespaceList = vi.fn().mockResolvedValue({ items: [{ name: 'default' }] })
    const wrapper = shallowMount(component, {
      global: {
        plugins: [pinia],
        mocks: { $t: (key: string) => key },
        directives: { permission: () => undefined },
        provide: {
          workloadApi: {}, k8sNetworkApi: {}, configMapApi: {}, secretApi: {},
          k8sNodeApi: {}, k8sEventApi: {}, k8sNsApi: { list: namespaceList },
          [apiName]: { list },
        },
        stubs: {
          'a-select': true, 'a-select-option': true, 'a-button': true,
          'a-table': true, 'a-tabs': true, 'a-tab-pane': true,
          'a-descriptions': true, 'a-descriptions-item': true,
          'a-empty': true, 'a-tag': true, 'a-modal': true,
          'a-card': { template: '<div><slot name="title"/><slot/></div>' },
          'a-space': { template: '<div><slot/></div>' },
          'a-alert': { props: ['message'], template: '<p>{{ message }}</p>' },
        },
      },
    })
    expect(wrapper.findComponent({ name: 'ClusterPicker' }).exists()).toBe(true)
    expect(wrapper.text()).toContain('k8s.cluster.no_cluster_selected')
    expect(list).not.toHaveBeenCalled()
    useK8sStore().setCluster(7, 'development')
    await flushPromises()
    expect(wrapper.text()).not.toContain('k8s.cluster.no_cluster_selected')
    expect(list.mock.calls.every(call => call[0] === 7)).toBe(true)
    expect(list).toHaveBeenCalled()
    useK8sStore().setCluster(9, 'production')
    await flushPromises()
    expect(list.mock.lastCall?.[0]).toBe(9)
    wrapper.unmount()
  })
})
