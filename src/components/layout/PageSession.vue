<template>
  <div ref="element" class="page-session" @scroll.passive="saveScroll">
    <slot />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, provide, ref } from 'vue'
import { pageStateKey } from '@/hooks/usePageState'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{ pageKey?: string }>()
const state = props.pageKey ? useWorkspaceStore().state(props.pageKey) : undefined
provide(pageStateKey, state)
const element = ref<HTMLElement>()
let observer: ResizeObserver | undefined
let restoring = true

function saveScroll() {
  if (!state || !element.value || restoring) return
  state.scrollTop = element.value.scrollTop
  state.scrollLeft = element.value.scrollLeft
}
function restoreScroll() {
  if (!element.value || !state) return
  element.value.scrollTop = state.scrollTop
  element.value.scrollLeft = state.scrollLeft
}
function stopRestoring() {
  restoring = false
  observer?.disconnect()
}
onMounted(() => {
  restoreScroll()
  // Lists load asynchronously; retry when their content grows, until the user
  // starts interacting. Observe content rather than the fixed scroll viewport.
  if (typeof ResizeObserver !== 'undefined' && element.value?.firstElementChild) {
    observer = new ResizeObserver(restoreScroll)
    observer.observe(element.value.firstElementChild)
  }
  for (const event of ['wheel', 'touchstart', 'pointerdown', 'keydown']) {
    element.value?.addEventListener(event, stopRestoring, { passive: true })
  }
})
onBeforeUnmount(() => { observer?.disconnect() })
</script>

<style scoped>
.page-session {
  height: 100%;
  min-height: 0;
  overflow: auto;
  padding: 8px;
  scrollbar-gutter: stable;
}
</style>
