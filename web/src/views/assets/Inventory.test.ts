import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { Button } from 'ant-design-vue'
import { describe, expect, it } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { inventoryPermissions, inventorySections } from './sections'
import Inventory from './Inventory.vue'

async function setup(permissions: string[] = [...inventoryPermissions]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.setPermissions(permissions)
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/assets/inventory', component: Inventory },
    ...inventorySections.map(section => ({ path: `/assets/inventory/${section.key}`, component: Inventory, meta: { assetsSection: section.key } })),
  ] })
  await router.push('/assets/inventory')
  await router.isReady()
  const wrapper = mount(Inventory, { global: {
    plugins: [pinia, router], mocks: { $t: (key: string) => key },
    components: { AButton: Button },
    stubs: { Accounts: true, VPCs: true, Firewalls: true, Instances: true, Databases: true, SyncRuns: true, 'a-empty': true },
  } })
  return { wrapper, router, auth }
}

describe('inventory page navigation', () => {
  it('shows six sections and opens the selected list', async () => {
    const { wrapper, router } = await setup()
    const buttons = wrapper.findAll('nav button')
    expect(buttons).toHaveLength(6)
    await buttons[2]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/assets/inventory/firewalls')
    expect(wrapper.findComponent({ name: 'Firewalls' }).exists()).toBe(true)
    expect(wrapper.findAll('nav [aria-current="page"]')).toHaveLength(1)
    wrapper.unmount()
  })

  it('opens the first allowed section and drops a revoked section immediately', async () => {
    const { wrapper, router, auth } = await setup(['assets:sync:read'])
    expect(wrapper.findAll('nav button')).toHaveLength(1)
    expect(wrapper.findComponent({ name: 'SyncRuns' }).exists()).toBe(true)
    await router.push('/assets/inventory/accounts')
    await flushPromises()
    expect(wrapper.findComponent({ name: 'Accounts' }).exists()).toBe(false)
    auth.setPermissions([])
    await flushPromises()
    expect(wrapper.findAll('nav button')).toHaveLength(0)
    expect(wrapper.findComponent({ name: 'SyncRuns' }).exists()).toBe(false)
    wrapper.unmount()
  })
})
