/* eslint-disable vue/one-component-per-file -- Router integration fixtures share one test harness. */
import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, onMounted, onUnmounted, ref } from 'vue'
import { createRouter, createMemoryHistory } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import { useWorkspaceStore } from '@/stores/workspace'
import { useTable } from '@/hooks/useTable'
import { usePageState } from '@/hooks/usePageState'
import type { MeMenuNode } from '@/types/api'
import WorkspaceTabs from './WorkspaceTabs.vue'
import WorkspaceView from './WorkspaceView.vue'

async function setup() {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.setActiveTokens('test', 'test')
  auth.setPermissions(['a:read', 'b:read'])
  useMenuStore().setTree(['a', 'b'].map(code => ({
    code, path: `/${code}`, name: `menu.${code}`, component: `${code}/List`, permission_code: `${code}:read`,
  } as MeMenuNode)))
  const unmounted = vi.fn()
  const fetcher = vi.fn().mockResolvedValue({ items: [{ id: 1 }], total: 100 })
  const Page = defineComponent({
    setup() {
      const search = ref('')
      usePageState({ search })
      const table = useTable({ fetcher })
      onMounted(table.reload)
      onUnmounted(unmounted)
      return { search, table }
    },
    template: '<div><input v-model="search"/><button class="page-next" @click="table.setPage(3)">Next</button></div>',
  })
  const router = createRouter({ history: createMemoryHistory(), routes: [{
    path: '/', component: WorkspaceView, children: [
      { path: 'a', component: Page, meta: { menuName: 'menu.a' } },
      { path: 'b', component: Page, meta: { menuName: 'menu.b' } },
      { path: 'a/:id', component: { template: '<p>Detail</p>' } },
      { path: 'workspace', component: { template: '<p>Empty</p>' } },
    ],
  }] })
  await router.push('/a')
  const wrapper = mount(defineComponent({
    components: { WorkspaceTabs }, template: '<WorkspaceTabs/><router-view/>',
  }), { global: {
    plugins: [pinia, router], mocks: { $t: (key: string) => key },
    stubs: {
      'a-tabs': { name: 'TabsControl', props: ['activeKey'], template: '<div><slot/><slot name="rightExtra"/></div>' },
      'a-tab-pane': { props: ['tab'], template: '<div>{{ tab }}</div>' },
      'a-dropdown': { template: '<div><slot/><slot name="overlay"/></div>' },
      'a-button': { template: '<button><slot/></button>' },
      'a-menu': { template: '<div><slot/></div>' },
      'a-menu-item': { template: '<button><slot/></button>' },
    },
  } })
  await flushPromises()
  return { wrapper, router, auth, workspace: useWorkspaceStore(), unmounted, fetcher }
}

describe('workspace navigation', () => {
  it('deduplicates menu tabs and restores filters/pagination while fetching fresh rows', async () => {
    const { wrapper, router, workspace, fetcher, unmounted } = await setup()
    await wrapper.get('input').setValue('production')
    await wrapper.get('.page-next').trigger('click')
    await router.push('/b')
    await flushPromises()
    expect(unmounted).toHaveBeenCalledTimes(1)
    expect(wrapper.get('input').element.value).toBe('')
    await router.push('/a')
    await flushPromises()
    expect(workspace.tabs.map(tab => tab.path)).toEqual(['/a', '/b'])
    expect(wrapper.get('input').element.value).toBe('production')
    expect(fetcher).toHaveBeenLastCalledWith({ page: 3, pageSize: 20, filters: {} })
    expect(Object.keys(workspace.states['/a']!.values)).not.toContain('table.items')
    wrapper.unmount()
  })

  it('keeps details under the parent tab and follows browser history', async () => {
    const { wrapper, router, workspace } = await setup()
    await router.push('/b')
    await router.push('/a/42')
    await flushPromises()
    expect(workspace.tabs.map(tab => tab.path)).toEqual(['/a', '/b'])
    expect(wrapper.findComponent({ name: 'TabsControl' }).props('activeKey')).toBe('/a')
    router.back()
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/b'))
    expect(wrapper.findComponent({ name: 'TabsControl' }).props('activeKey')).toBe('/b')
    wrapper.unmount()
  })

  it('does not discard tabs when navigation on close is rejected', async () => {
    const { wrapper, router, workspace } = await setup()
    const remove = router.beforeEach(() => false)
    const tabs = wrapper.findComponent(WorkspaceTabs)
    const control = tabs.findComponent({ name: 'TabsControl' })
    control.vm.$emit('edit', '/a', 'remove')
    await flushPromises()
    expect(workspace.tabs.map(tab => tab.path)).toEqual(['/a'])
    remove()
    control.vm.$emit('edit', '/a', 'remove')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/workspace')
    expect(workspace.tabs).toEqual([])
    expect(workspace.states['/a']).toBeUndefined()
    wrapper.unmount()
  })

  it('removes revoked tabs and clears session state on logout', async () => {
    const { wrapper, router, auth, workspace } = await setup()
    await router.push('/b')
    auth.setPermissions(['b:read'])
    await flushPromises()
    expect(workspace.tabs.map(tab => tab.path)).toEqual(['/b'])
    expect(workspace.states['/a']).toBeUndefined()
    auth.reset()
    expect(workspace.tabs).toEqual([])
    expect(workspace.states).toEqual({})
    wrapper.unmount()
  })

  it('closes other tabs, then all tabs, and starts reopened pages with fresh state', async () => {
    const { wrapper, router, workspace } = await setup()
    await wrapper.get('input').setValue('old filter')
    await router.push('/b')
    await flushPromises()
    const buttons = wrapper.findComponent(WorkspaceTabs).findAll('button')
    await buttons.find(button => button.text() === 'workspace.close_others')!.trigger('click')
    expect(workspace.tabs.map(tab => tab.path)).toEqual(['/b'])
    expect(workspace.states['/a']).toBeUndefined()
    await buttons.find(button => button.text() === 'workspace.close_all')!.trigger('click')
    await flushPromises()
    expect(workspace.tabs).toEqual([])
    expect(router.currentRoute.value.path).toBe('/workspace')
    await router.push('/a')
    await flushPromises()
    expect(wrapper.get('input').element.value).toBe('')
    wrapper.unmount()
  })
})
