import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { expect, it } from 'vitest'
import { useWorkspaceStore } from '@/stores/workspace'
import PageSession from './PageSession.vue'

it('restores scroll per page without retaining component content', async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  const mountPage = (pageKey: string) => mount(PageSession, {
    props: { pageKey }, global: { plugins: [pinia] }, slots: { default: '<div>Fresh data</div>' },
  })
  const first = mountPage('/a')
  await first.trigger('wheel')
  first.element.scrollTop = 240
  first.element.scrollLeft = 30
  await first.trigger('scroll')
  first.unmount()
  const second = mountPage('/b')
  expect(second.element.scrollTop).toBe(0)
  second.unmount()
  const restored = mountPage('/a')
  expect(restored.element.scrollTop).toBe(240)
  expect(restored.element.scrollLeft).toBe(30)
  expect(useWorkspaceStore().states['/a']?.values).toEqual({})
  restored.unmount()
})
