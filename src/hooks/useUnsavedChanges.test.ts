import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent, ref } from 'vue'
import { createMemoryHistory, createRouter, isNavigationFailure } from 'vue-router'
import { expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { useUnsavedChanges } from './useUnsavedChanges'

vi.mock('@/hooks/useI18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

it('allows cancelling navigation, stops warning after save, and removes the guard on unmount', async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().setActiveTokens('test', 'test')
  const dirty = ref(true)
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/a', component: { template: '<div/>' } },
    { path: '/b', component: { template: '<div/>' } },
  ] })
  await router.push('/a')
  const wrapper = mount(defineComponent({
    setup() { useUnsavedChanges(() => dirty.value); return {} },
    template: '<div/>',
  }), { global: { plugins: [pinia, router] } })
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
  try {
    expect(isNavigationFailure(await router.push('/b'))).toBe(true)
    expect(router.currentRoute.value.path).toBe('/a')
    expect(confirm).toHaveBeenCalledOnce()
    dirty.value = false
    await router.push('/b')
    expect(router.currentRoute.value.path).toBe('/b')
    expect(confirm).toHaveBeenCalledOnce()
    dirty.value = true
    wrapper.unmount()
    await router.push('/a')
    expect(router.currentRoute.value.path).toBe('/a')
    expect(confirm).toHaveBeenCalledOnce()
  } finally { confirm.mockRestore() }
})
