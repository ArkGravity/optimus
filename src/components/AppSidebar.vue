<template>
  <div class="app-sidebar">
    <div class="logo">
      <img src="/optimus-logo.png" alt="Optimus" width="32" height="32" />
      <span v-if="!collapsed">Optimus</span>
    </div>
    <a-menu
      class="sidebar-menu"
      :selected-keys="[currentKey]"
      :open-keys="openKeys"
      mode="inline"
      :theme="app.theme === 'dark' ? 'light' : 'dark'"
      :items="items"
      @click="onClick"
      @open-change="onOpenChange"
    >
      <template #expandIcon="{ isOpen }">
        <DownOutlined v-if="isOpen" class="submenu-chevron" />
        <RightOutlined v-else class="submenu-chevron" />
      </template>
    </a-menu>
    <div class="sidebar-footer">
      <button
        type="button"
        class="sidebar-toggle"
        :aria-label="collapsed ? $t('common.expand_sidebar') : $t('common.collapse_sidebar')"
        :title="collapsed ? $t('common.expand_sidebar') : $t('common.collapse_sidebar')"
        :aria-expanded="!collapsed"
        @click="$emit('toggle')"
      >
        <MenuOutlined aria-hidden="true" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { DownOutlined, MenuOutlined, RightOutlined } from '@ant-design/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import { useMenuStore } from '@/stores/menu'
import { useAppStore } from '@/stores/app'
import { useI18n } from '@/hooks/useI18n'
import type { MeMenuNode } from '@/types/api'
import type { ItemType } from 'ant-design-vue'

defineProps<{ collapsed: boolean }>()
defineEmits<{ toggle: [] }>()
const app = useAppStore()
// The light menu variant inherits the global dark algorithm's neutral tokens;
// Ant Design's dark menu variant uses a separate, fixed navy palette.
const menu = useMenuStore()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

function buildItems(nodes: MeMenuNode[]): ItemType[] {
  return nodes.map(n => {
    const base = { key: n.code, label: t(n.name), title: t(n.name) }
    if (n.children?.length) return { ...base, children: buildItems(n.children) } as ItemType
    return base as ItemType
  })
}

const items = computed(() => buildItems(menu.tree))

const codeByPath = computed(() => {
  const map = new Map<string, string>()
  const walk = (ns: MeMenuNode[]) => {
    for (const n of ns) {
      if (n.path) map.set(n.path, n.code)
      if (n.children?.length) walk(n.children)
    }
  }
  walk(menu.tree)
  return map
})

const currentKey = computed(() => [...codeByPath.value.entries()]
  .filter(([path]) => route.path === path || route.path.startsWith(`${path}/`))
  .sort(([a], [b]) => b.length - a.length)[0]?.[1] ?? '')
const openKeys = ref<string[]>([])
watch(currentKey, key => {
  function ancestors(nodes: MeMenuNode[], parents: string[] = []): string[] | undefined {
    for (const node of nodes) {
      if (node.code === key) return parents
      const found = node.children && ancestors(node.children, [...parents, node.code])
      if (found) return found
    }
  }
  openKeys.value = [...new Set([...openKeys.value, ...(ancestors(menu.tree) ?? [])])]
}, { immediate: true })

function onClick({ key }: { key: string }) {
  const node = findNode(menu.tree, key)
  if (node?.path) router.push(node.path)
}

function onOpenChange(keys: (string | number)[]) {
  openKeys.value = keys.map(k => String(k))
}

function findNode(ns: MeMenuNode[], code: string): MeMenuNode | undefined {
  for (const n of ns) {
    if (n.code === code) return n
    const found = n.children ? findNode(n.children, code) : undefined
    if (found) return found
  }
}
</script>

<style scoped lang="scss">
.app-sidebar {
  position: sticky;
  top: 0;
  height: 100vh;
  height: 100dvh;
  background: var(--sidebar-bg);
  color: var(--sidebar-text);
  display: flex;
  flex-direction: column;
}
.logo {
  height: 56px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: inherit;
  font-weight: 600;
  letter-spacing: 1px;
  gap: 10px;
}
.sidebar-menu {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  border-inline-end: 0;
}
.sidebar-footer {
  flex-shrink: 0;
  padding: 8px 16px;
  border-top: 1px solid var(--sidebar-border);
}
.sidebar-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 40px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: inherit;
  font-size: 18px;
  cursor: pointer;

  &:hover {
    background: rgba(255, 255, 255, 0.08);
  }
  &:focus-visible {
    outline: 2px solid currentColor;
    outline-offset: -2px;
  }
}
.submenu-chevron {
  position: absolute;
  right: 16px;
  top: 50%;
  transform: translateY(-50%);
  font-size: 12px;
}
:deep(.ant-menu-title-content) {
  white-space: normal;
  line-height: 1.5;
}
:deep(.ant-menu-item),
:deep(.ant-menu-submenu-title) {
  height: auto;
  min-height: 40px;
  padding-top: 8px;
  padding-bottom: 8px;
}
</style>
