import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { Menu } from 'ant-design-vue'
import { describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import AppSidebar from './AppSidebar.vue'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
describe('container sidebar', () => {
  it('selects Kubernetes for resource URLs, exposes only two built-in children and prunes revoked entries', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const auth = useAuthStore()
    auth.setPermissions(['k8s:storage:read', 'docker:environment:read'])
    useMenuStore().setTree([{
      id: 1, code: 'containers', name: 'menu.containers_group', path: '/containers', component: '', icon: '', sort_order: 0, hidden: false,
      children: [
        { id: 2, code: 'containers.kubernetes', name: 'Kubernetes', path: '/containers/kubernetes', component: 'containers/Kubernetes', icon: '', sort_order: 0, hidden: false },
        { id: 3, code: 'containers.docker', name: 'Docker', path: '/containers/docker', component: 'containers/Docker', permission_code: 'docker:environment:read', icon: '', sort_order: 1, hidden: false },
      ],
    }])
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }] })
    await router.push('/containers/kubernetes/storage')
    const wrapper = mount(AppSidebar, { props: { collapsed: false }, global: { plugins: [pinia, router], components: { AMenu: Menu }, mocks: { $t: (key: string) => key } } })
    await flushPromises()
    expect(wrapper.findAll('.ant-menu-item')).toHaveLength(2)
    expect(wrapper.get('.ant-menu-item-selected').text()).toBe('Kubernetes')
    expect(wrapper.text()).toContain('Docker')
    auth.setPermissions(['docker:environment:read'])
    await flushPromises()
    expect(wrapper.findAll('.ant-menu-item')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('Kubernetes')
    auth.setPermissions([])
    await flushPromises()
    expect(wrapper.find('.ant-menu-submenu').exists()).toBe(false)
    wrapper.unmount()
  })
})
