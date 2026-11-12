import { onBeforeUnmount, watch, type Ref } from 'vue'
import { useK8sStore } from '@/stores/k8s'
import { useAuthStore } from '@/stores/auth'

// Prevent a response from a previous resource, cluster or session reopening UI.
export function useResourceScope(kind: Ref<string>) {
  const k8s = useK8sStore()
  const auth = useAuthStore()
  let generation = 0
  const channels = new Map<string, number>()
  watch([() => k8s.currentClusterId, () => k8s.currentNamespace, kind, () => auth.permissions], () => generation++, { flush: 'sync', deep: true })
  onBeforeUnmount(() => generation++)
  return (channel = 'list') => {
    const captured = generation
    const request = (channels.get(channel) ?? 0) + 1
    channels.set(channel, request)
    return () => captured === generation && channels.get(channel) === request
  }
}

export function matchesResource(row: { name?: string; namespace?: string }, search: string) {
  const query = search.trim().toLocaleLowerCase()
  return !query || `${row.name ?? ''} ${row.namespace ?? ''}`.toLocaleLowerCase().includes(query)
}
