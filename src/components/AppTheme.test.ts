import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { computed, defineComponent, nextTick } from 'vue'
import { ConfigProvider, theme } from 'ant-design-vue'
import { expect, it } from 'vitest'
import { useAppStore } from '@/stores/app'
import AppTheme from './AppTheme.vue'

it('updates custom surface and text colors with the component theme', async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  const app = useAppStore()
  const wrapper = mount(defineComponent({
    components: { ConfigProvider, AppTheme },
    setup: () => ({ algorithm: computed(() => app.theme === 'dark' ? theme.darkAlgorithm : theme.defaultAlgorithm) }),
    template: '<ConfigProvider :theme="{ algorithm }"><AppTheme><div /></AppTheme></ConfigProvider>',
  }), { global: { plugins: [pinia] } })
  const root = document.documentElement
  expect(root.style.getPropertyValue('--ant-color-bg-container')).toBe('#ffffff')
  app.setTheme('dark')
  await nextTick()
  expect(root.style.getPropertyValue('--ant-color-bg-container')).toBe('#141414')
  expect(root.style.getPropertyValue('--ant-color-bg-layout')).toBe('#000000')
  expect(root.style.getPropertyValue('--ant-color-text')).toContain('255, 255, 255')
  expect(root.style.colorScheme).toBe('dark')
  app.setTheme('light')
  await nextTick()
  expect(root.style.getPropertyValue('--ant-color-bg-container')).toBe('#ffffff')
  wrapper.unmount()
})
