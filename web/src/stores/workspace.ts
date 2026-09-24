import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface WorkspaceTab {
  path: string
  fullPath: string
  title: string
  permission?: string
}
export interface PageState {
  values: Record<string, unknown>
  scrollTop: number
  scrollLeft: number
}

// Session-only navigation state. Never retain API results, forms or secrets.
export const useWorkspaceStore = defineStore('workspace', () => {
  const tabs = ref<WorkspaceTab[]>([])
  const states = ref<Record<string, PageState>>({})

  function visit(tab: WorkspaceTab) {
    const existing = tabs.value.find(item => item.path === tab.path)
    if (existing) Object.assign(existing, tab)
    else tabs.value.push(tab)
  }
  function state(path: string): PageState {
    return states.value[path] ??= { values: {}, scrollTop: 0, scrollLeft: 0 }
  }
  function remove(paths: string[]) {
    tabs.value = tabs.value.filter(tab => !paths.includes(tab.path))
    for (const path of paths) delete states.value[path]
  }
  function reset() {
    tabs.value = []
    states.value = {}
  }
  return { tabs, states, visit, state, remove, reset }
})
