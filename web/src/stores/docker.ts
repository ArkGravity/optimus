import { defineStore } from 'pinia'
import { ref } from 'vue'

/**
 * useDockerStore holds the Docker host shared by the Images and Containers
 * sections. A host is only ever selected by an explicit user choice; nothing
 * here or in HostPicker auto-selects one.
 */
export const useDockerStore = defineStore('docker', () => {
  const selectedHostId = ref<number | null>(null)
  const selectedHostName = ref('')

  function setHost(id: number, name: string) {
    selectedHostId.value = id
    selectedHostName.value = name
  }

  /** Clear the selection, or only when it currently points at `id`. */
  function clearHost(id?: number) {
    if (id !== undefined && id !== selectedHostId.value) return
    selectedHostId.value = null
    selectedHostName.value = ''
  }

  return { selectedHostId, selectedHostName, setHost, clearHost, reset: () => clearHost() }
}, {
  persist: {
    storage: typeof window !== 'undefined' ? window.localStorage : undefined,
    pick: ['selectedHostId', 'selectedHostName'],
  },
})
