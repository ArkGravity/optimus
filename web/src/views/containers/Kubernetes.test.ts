import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import { Button } from 'ant-design-vue'
import { useAuthStore } from '@/stores/auth'
import { kubernetesPermissions, kubernetesSections } from './sections'
import Kubernetes from './Kubernetes.vue'
import DefaultLayout from '@/layouts/DefaultLayout.vue'

async function setup(permissions = kubernetesPermissions) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.setPermissions(permissions)
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/containers/kubernetes', component: Kubernetes },
    ...kubernetesSections.map(section => ({ path: `/containers/kubernetes/${section.key}`, component: Kubernetes, meta: { kubernetesSection: section.key } })),
  ] })
  await router.push('/containers/kubernetes')
  await router.isReady()
  const wrapper = mount(Kubernetes, { global: {
    plugins: [pinia, router], mocks: { $t: (key: string) => key },
    components: { AButton: Button },
    stubs: { Clusters: true, ClusterResources: true, Workloads: true, Network: true, Storage: true, Configs: true, 'a-empty': true },
  } })
  return { wrapper, router, auth }
}

describe('Kubernetes page navigation', () => {
  it('has seven functional buttons, with Nodes and Namespaces as distinct pages', async () => {
    const { wrapper, router } = await setup()
    const buttons = wrapper.findAll('nav button')
    expect(buttons).toHaveLength(7)
    await buttons[1]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/containers/kubernetes/nodes')
    expect(wrapper.findComponent({ name: 'ClusterResources' }).props('section')).toBe('nodes')
    await buttons[2]!.trigger('click')
    await flushPromises()
    expect(wrapper.findComponent({ name: 'ClusterResources' }).props('section')).toBe('namespaces')
    expect(wrapper.findAll('nav [aria-current="page"]')).toHaveLength(1)
    wrapper.unmount()
  })
  it('supports a Secret-only role and removes revoked resources immediately', async () => {
    const { wrapper, router, auth } = await setup(['k8s:secret:read'])
    expect(wrapper.findAll('nav button')).toHaveLength(1)
    expect(wrapper.findComponent({ name: 'Configs' }).exists()).toBe(true)
    await router.push('/containers/kubernetes/storage')
    await flushPromises()
    expect(wrapper.findComponent({ name: 'Storage' }).exists()).toBe(false)
    auth.setPermissions([])
    await flushPromises()
    expect(wrapper.findAll('nav button')).toHaveLength(0)
    expect(wrapper.findComponent({ name: 'Configs' }).exists()).toBe(false)
    wrapper.unmount()
  })
  it('renders the layout without a workspace tab bar', async () => {
    const pinia = createPinia()
    const wrapper = mount(DefaultLayout, { global: { plugins: [pinia], stubs: {
      AppSidebar: true, AppHeader: true, 'router-view': true,
      'a-layout': { template: '<main><slot /></main>' },
      'a-layout-header': { template: '<header><slot /></header>' },
      'a-layout-content': { template: '<article><slot /></article>' },
      'a-layout-sider': true,
    } } })
    expect(wrapper.find('article router-view-stub').exists()).toBe(true)
    expect(wrapper.find('.workspace-tabs, .ant-tabs-nav-wrap, workspace-tabs-stub').exists()).toBe(false)
    wrapper.unmount()
  })
})
