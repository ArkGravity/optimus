import { defineStore } from 'pinia'
import { ref } from 'vue'

export interface PageState {
  values: Record<string, unknown>
  scrollTop: number
  scrollLeft: number
}

// Session-only navigation state. Never retain API results, forms or secrets.
export const useWorkspaceStore = defineStore('workspace', () => {
  const states = ref<Record<string, PageState>>({})
  function state(path: string): PageState {
    return states.value[path] ??= { values: {}, scrollTop: 0, scrollLeft: 0 }
  }
  function reset() {
    states.value = {}
  }
  return { states, state, reset }
})
