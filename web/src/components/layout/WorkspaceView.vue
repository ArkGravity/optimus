<template>
  <router-view v-slot="{ Component, route: current }">
    <PageSession :key="current.path" :page-key="current.path">
      <component :is="Component" />
    </PageSession>
  </router-view>
</template>

<script setup lang="ts">
import PageSession from './PageSession.vue'
import { watch } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useMenuStore } from '@/stores/menu'
import { useWorkspaceStore } from '@/stores/workspace'

// Discard retained controls when permissions or available menus change.
const auth = useAuthStore()
const menus = useMenuStore()
const workspace = useWorkspaceStore()
watch([() => auth.permissions, () => menus.tree], () => workspace.reset(), { deep: true })
</script>
