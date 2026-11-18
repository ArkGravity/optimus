import { onBeforeUnmount, watch } from 'vue'
import { useDockerStore } from '@/stores/docker'
import { useAuthStore } from '@/stores/auth'
import { isBizError } from '@/utils/http-error'

// Discard responses that arrive after the host, permissions or page changed.
export function useHostScope() {
  const docker = useDockerStore()
  const auth = useAuthStore()
  let generation = 0
  const channels = new Map<string, number>()
  watch([() => docker.selectedHostId, () => auth.permissions], () => generation++, { flush: 'sync', deep: true })
  onBeforeUnmount(() => generation++)
  return (channel = 'list') => {
    const captured = generation
    const request = (channels.get(channel) ?? 0) + 1
    channels.set(channel, request)
    return () => captured === generation && channels.get(channel) === request
  }
}

export function errorText(error: unknown, fallback: string): string {
  return isBizError(error) && error.message ? error.message : fallback
}

export function formatTime(iso?: string): string {
  if (!iso) return '-'
  const date = new Date(iso)
  return Number.isNaN(date.getTime()) ? iso : date.toLocaleString()
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000
    unit++
  }
  return `${unit === 0 ? value : value.toFixed(1)} ${units[unit]}`
}
