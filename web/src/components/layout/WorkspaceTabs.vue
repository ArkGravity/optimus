<template>
  <nav v-if="workspace.tabs.length" class="workspace-tabs" :aria-label="$t('workspace.tabs')">
    <a-tabs
      :active-key="activePath"
      type="editable-card"
      hide-add
      @change="activate"
      @edit="onEdit"
    >
      <a-tab-pane v-for="tab in workspace.tabs" :key="tab.path" :tab="$t(tab.title)" closable />
      <template #rightExtra>
        <a-dropdown :trigger="['click']">
          <a-button type="text" :aria-label="$t('workspace.actions')">
            <MoreOutlined />
          </a-button>
          <template #overlay>
            <a-menu>
              <a-menu-item key="others" @click="closeOthers">{{ $t('workspace.close_others') }}</a-menu-item>
              <a-menu-item key="all" @click="closeAll">{{ $t('workspace.close_all') }}</a-menu-item>
            </a-menu>
          </template>
        </a-dropdown>
      </template>
    </a-tabs>
  </nav>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MoreOutlined } from '@ant-design/icons-vue'
import { useWorkspaceStore, type WorkspaceTab } from '@/stores/workspace'
import { useMenuStore } from '@/stores/menu'
import { useAuthStore } from '@/stores/auth'
import type { MeMenuNode } from '@/types/api'

const workspace = useWorkspaceStore()
const menus = useMenuStore()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const available = computed(() => {
  const result: WorkspaceTab[] = []
  function walk(nodes: MeMenuNode[]) {
    for (const node of nodes) {
      if (node.path && node.component && (!node.permission_code || auth.permissions.includes(node.permission_code))) {
        result.push({ path: node.path, fullPath: node.path, title: node.name, permission: node.permission_code || undefined })
      }
      if (node.children) walk(node.children)
    }
  }
  walk(menus.tree)
  result.push({ path: '/profile', fullPath: '/profile', title: 'common.profile' })
  return result
})
const currentMenu = computed(() => available.value
  .filter(tab => route.path === tab.path || route.path.startsWith(`${tab.path}/`))
  .sort((a, b) => b.path.length - a.path.length)[0])
const activePath = computed(() => currentMenu.value?.path ?? '')

watch([() => route.fullPath, available], () => {
  workspace.remove(workspace.tabs.filter(tab => !available.value.some(item => item.path === tab.path)).map(tab => tab.path))
  if (!auth.accessToken || !currentMenu.value || route.meta.layout === 'blank') return
  const menu = currentMenu.value
  const existing = workspace.tabs.find(tab => tab.path === menu.path)
  workspace.visit({ ...menu, fullPath: route.path === menu.path ? route.fullPath : existing?.fullPath ?? menu.path })
}, { immediate: true })

async function activate(key: string | number) {
  const tab = workspace.tabs.find(item => item.path === key)
  if (tab) await router.push(tab.fullPath)
}
async function close(path: string) {
  const index = workspace.tabs.findIndex(tab => tab.path === path)
  if (index < 0) return
  if (activePath.value === path) {
    const next = workspace.tabs[index + 1] ?? workspace.tabs[index - 1]
    const failure = await router.push(next?.fullPath ?? '/workspace')
    if (failure) return
  }
  workspace.remove([path])
}
function onEdit(key: string | number | MouseEvent, action: string) {
  if (action === 'remove' && typeof key !== 'object') void close(String(key))
}
function closeOthers() {
  workspace.remove(workspace.tabs.filter(tab => tab.path !== activePath.value).map(tab => tab.path))
}
async function closeAll() {
  if (route.path !== '/workspace' && await router.push('/workspace')) return
  workspace.reset()
}
</script>

<style scoped lang="scss">
.workspace-tabs {
  flex-shrink: 0;
  min-width: 0;
  padding: 6px 8px 0;
  background: var(--ant-color-bg-container);
  border-bottom: 1px solid var(--ant-color-border);
  :deep(.ant-tabs-nav) { margin: 0; }
  :deep(.ant-tabs-content-holder) { display: none; }
  :deep(.ant-tabs-tab) { padding: 6px 12px; }
}
</style>
