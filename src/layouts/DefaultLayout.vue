<template>
  <a-layout class="default-layout">
    <a-layout-sider
      v-model:collapsed="collapsed"
      class="sidebar"
      :class="{ 'sidebar-dark': app.theme === 'dark' }"
      :width="app.locale === 'en-US' ? 280 : 224"
      :trigger="null"
      collapsible
    >
      <AppSidebar :collapsed="collapsed" @toggle="collapsed = !collapsed" />
    </a-layout-sider>
    <a-layout class="main-layout">
      <a-layout-header class="header">
        <AppHeader />
      </a-layout-header>
      <WorkspaceTabs />
      <a-layout-content class="content">
        <router-view />
      </a-layout-content>
    </a-layout>
  </a-layout>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import AppSidebar from '@/components/AppSidebar.vue'
import AppHeader from '@/components/AppHeader.vue'
import WorkspaceTabs from '@/components/layout/WorkspaceTabs.vue'

const app = useAppStore()
const collapsed = ref(app.sidebarCollapsed)
watch(collapsed, v => {
  if (v !== app.sidebarCollapsed) app.toggleSidebar()
})
watch(
  () => app.sidebarCollapsed,
  v => {
    collapsed.value = v
  }
)
</script>

<style scoped lang="scss">
.default-layout {
  height: 100vh;
  height: 100dvh;
  overflow: hidden;
}
.sidebar {
  --sidebar-bg: #001529;
  --sidebar-text: rgba(255, 255, 255, 0.85);
  --sidebar-border: rgba(255, 255, 255, 0.16);
  background: var(--sidebar-bg);
}
.sidebar-dark {
  --sidebar-bg: var(--ant-color-bg-container);
  --sidebar-text: var(--ant-color-text);
  --sidebar-border: var(--ant-color-border);
}
.header {
  background: var(--ant-color-bg-container);
  height: 56px;
  line-height: normal;
  padding: 0 16px;
  border-bottom: 1px solid var(--ant-color-border, #f0f0f0);
}
.main-layout {
  min-width: 0;
  min-height: 0;
}
.content {
  min-height: 0;
  overflow: hidden;
  background: var(--ant-color-bg-layout, #f5f5f5);
}
</style>
