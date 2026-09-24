<template>
  <slot />
</template>

<script setup lang="ts">
import { watchEffect } from 'vue'
import { theme } from 'ant-design-vue'
import { useAppStore } from '@/stores/app'

const { token } = theme.useToken()
const app = useAppStore()

// Bridge Ant Design's computed tokens to custom styles and teleported content.
watchEffect(() => {
  const root = document.documentElement
  const colors = {
    'color-bg-layout': token.value.colorBgLayout,
    'color-bg-container': token.value.colorBgContainer,
    'color-border': token.value.colorBorder,
    'color-text': token.value.colorText,
    'color-text-secondary': token.value.colorTextSecondary,
    'color-text-tertiary': token.value.colorTextTertiary,
    'color-error': token.value.colorError
  }
  for (const [name, value] of Object.entries(colors)) {
    root.style.setProperty(`--ant-${name}`, value)
  }
  root.style.colorScheme = app.theme
  root.lang = app.locale
})
</script>

<style lang="scss">
body {
  background: var(--ant-color-bg-layout);
  color: var(--ant-color-text);
}
</style>
