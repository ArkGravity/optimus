import { getCurrentInstance, inject, watch, type InjectionKey, type Ref } from 'vue'
import type { PageState } from '@/stores/workspace'

export const pageStateKey: InjectionKey<PageState | undefined> = Symbol('page-state')

// Call only with explicitly approved navigation/filter fields, never form data.
export function usePageState(fields: Record<string, Ref>, prefix = 'controls') {
  const state = getCurrentInstance() ? inject(pageStateKey, undefined) : undefined
  if (!state) return
  for (const [name, field] of Object.entries(fields)) {
    const key = `${prefix}.${name}`
    if (Object.prototype.hasOwnProperty.call(state.values, key)) field.value = state.values[key]
    watch(field, value => { state.values[key] = value }, { deep: true, immediate: true, flush: 'sync' })
  }
}
