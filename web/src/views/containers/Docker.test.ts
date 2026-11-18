import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'
import { Button } from 'ant-design-vue'
import { useAuthStore } from '@/stores/auth'
import { dockerPermissions, dockerSections } from './sections'
import Docker from './Docker.vue'

async function setup(permissions: string[] = dockerPermissions) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.setPermissions(permissions)
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/containers/docker', component: Docker },
    ...dockerSections.map(section => ({ path: `/containers/docker/${section.key}`, component: Docker, meta: { dockerSection: section.key } })),
  ] })
  await router.push('/containers/docker')
  await router.isReady()
  const wrapper = mount(Docker, { global: {
    plugins: [pinia, router], mocks: { $t: (key: string) => key },
    components: { AButton: Button },
    stubs: { Hosts: true, Images: true, Containers: true, 'a-empty': true },
  } })
  return { wrapper, router, auth }
}

describe('Docker page navigation', () => {
  it('gates every section by the read permission only', () => {
    expect(dockerSections.map(section => section.key)).toEqual(['hosts', 'images', 'containers'])
    expect(dockerPermissions).toEqual(['docker:environment:read'])
  })

  it('lands on Hosts without changing the URL and switches sections by button', async () => {
    const { wrapper, router } = await setup()
    const buttons = wrapper.findAll('nav button')
    expect(buttons).toHaveLength(3)
    expect(router.currentRoute.value.path).toBe('/containers/docker')
    expect(wrapper.findComponent({ name: 'Hosts' }).exists()).toBe(true)
    await buttons[1]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/containers/docker/images')
    expect(wrapper.findComponent({ name: 'Images' }).exists()).toBe(true)
    await buttons[2]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/containers/docker/containers')
    expect(wrapper.findComponent({ name: 'Containers' }).exists()).toBe(true)
    expect(wrapper.findAll('nav [aria-current="page"]')).toHaveLength(1)
    wrapper.unmount()
  })

  it('removes every section immediately when the read permission is revoked', async () => {
    const { wrapper, auth } = await setup(['docker:environment:read', 'docker:environment:write'])
    expect(wrapper.findAll('nav button')).toHaveLength(3)
    auth.setPermissions(['docker:environment:write'])
    await flushPromises()
    expect(wrapper.findAll('nav button')).toHaveLength(0)
    expect(wrapper.findComponent({ name: 'Hosts' }).exists()).toBe(false)
    wrapper.unmount()
  })
})
