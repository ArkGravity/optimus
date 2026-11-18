<template>
  <a-space class="host-picker" :size="4">
    <a-select
      :value="docker.selectedHostId ?? undefined"
      :options="options"
      :loading="loading"
      :placeholder="$t('docker.host.select')"
      :aria-label="$t('docker.host.select')"
      :not-found-content="$t('docker.host.none')"
      option-filter-prop="label"
      show-search
      allow-clear
      style="width: 260px"
      @change="onChange"
    />
    <a-button :loading="loading" :title="$t('common.refresh')" :aria-label="$t('common.refresh')" @click="refresh">
      <template #icon><ReloadOutlined /></template>
    </a-button>
  </a-space>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from 'vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import { useDockerStore } from '@/stores/docker'
import type { DockerHostApi } from '@/api/docker/host'
import type { DockerHost } from '@/types/docker'

const docker = useDockerStore()
const hostApi = inject<DockerHostApi>('dockerHostApi')!
const hosts = ref<DockerHost[]>([])
const loading = ref(false)
const options = computed(() => hosts.value.map(host => ({ value: host.id, label: host.name })))

async function refresh() {
  loading.value = true
  try {
    const result = await hostApi.list({ page_size: 200 })
    hosts.value = result.items
    const selected = docker.selectedHostId
    if (selected === null) return
    const match = result.items.find(host => host.id === selected)
    if (match) {
      if (match.name !== docker.selectedHostName) docker.setHost(match.id, match.name)
    } else if (result.total <= result.items.length) {
      // The persisted host was deleted elsewhere; drop it instead of guessing another.
      docker.clearHost(selected)
    }
  } catch {
    // The page shows request errors; the picker keeps the last known options.
  } finally {
    loading.value = false
  }
}

function onChange(value: unknown) {
  const host = hosts.value.find(item => item.id === value)
  if (host) docker.setHost(host.id, host.name)
  else docker.clearHost()
}

onMounted(() => { void refresh() })
</script>
