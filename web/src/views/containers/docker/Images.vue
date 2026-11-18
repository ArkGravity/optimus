<template>
  <a-card class="resource-card" :title="$t('docker.sections.images')">
    <a-space class="resource-toolbar" wrap>
      <HostPicker />
      <a-input v-model:value="search" allow-clear style="width: 240px" :placeholder="$t('docker.image.search')" />
      <a-button :loading="loading" :disabled="docker.selectedHostId === null" @click="load">
        <template #icon><ReloadOutlined /></template>
        {{ $t('common.refresh') }}
      </a-button>
      <a-button v-permission="WRITE" class="prune-images" danger :disabled="docker.selectedHostId === null" @click="openPrune">
        <template #icon><ClearOutlined /></template>
        {{ $t('docker.image.prune') }}
      </a-button>
    </a-space>
    <a-alert v-if="docker.selectedHostId === null" type="info" show-icon :message="$t('docker.host.no_host_selected')" />
    <a-table
      v-else
      :columns="columns"
      :data-source="filtered"
      :loading="loading"
      :pagination="{ pageSize: 20, showSizeChanger: true }"
      :scroll="{ x: 'max-content' }"
      row-key="id"
      size="small"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'tags'">
          <div v-for="tag in displayTags(record)" :key="tag">{{ tag }}</div>
        </template>
        <template v-else-if="column.key === 'short_id'"><code>{{ record.short_id }}</code></template>
        <template v-else-if="column.key === 'size'">{{ formatBytes(record.size) }}</template>
        <template v-else-if="column.key === 'created'">{{ formatTime(record.created) }}</template>
        <template v-else-if="column.key === 'actions'">
          <a-popconfirm @confirm="remove(record)" @open-change="(open: boolean) => { if (open) force = false }">
            <template #title>
              <div>{{ $t('docker.image.confirm_delete', { name: displayTags(record)[0] }) }}</div>
              <a-checkbox v-model:checked="force">{{ $t('docker.image.force') }}</a-checkbox>
            </template>
            <a v-permission="WRITE" class="delete-image danger" :class="{ disabled: busy[record.id] }">{{ $t('common.delete') }}</a>
          </a-popconfirm>
        </template>
      </template>
    </a-table>

    <a-modal
      v-model:open="pruneOpen"
      :title="$t('docker.image.prune_title')"
      :ok-text="$t('docker.image.prune')"
      :ok-button-props="{ danger: true }"
      :confirm-loading="pruning"
      @ok="prune"
    >
      <p>{{ $t('docker.image.prune_body', { host: docker.selectedHostName }) }}</p>
      <a-checkbox v-model:checked="pruneAll">{{ $t('docker.image.prune_all') }}</a-checkbox>
    </a-modal>
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { ClearOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { useI18n } from '@/hooks/useI18n'
import { usePageState } from '@/hooks/usePageState'
import { useAuthStore } from '@/stores/auth'
import { useDockerStore } from '@/stores/docker'
import HostPicker from './HostPicker.vue'
import { errorText, formatBytes, formatTime, useHostScope } from './scope'
import type { DockerImageApi } from '@/api/docker/image'
import type { DockerImage } from '@/types/docker'

const WRITE = 'docker:environment:write'
const { t } = useI18n()
const imageApi = inject<DockerImageApi>('dockerImageApi')!
const docker = useDockerStore()
const auth = useAuthStore()
const captureScope = useHostScope()
const search = ref('')
usePageState({ search })
const items = ref<DockerImage[]>([])
const loading = ref(false)
const busy = reactive<Record<string, boolean>>({})
const force = ref(false)
const pruneOpen = ref(false)
const pruneAll = ref(false)
const pruning = ref(false)

const columns = computed(() => [
  { key: 'tags', title: t('docker.image.repo_tag') },
  { key: 'short_id', title: t('docker.image.id') },
  { key: 'size', title: t('docker.image.size') },
  { key: 'created', title: t('common.created_at') },
  { key: 'actions', title: t('common.actions') },
])
const filtered = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return query ? items.value.filter(image => `${image.id} ${image.repo_tags.join(' ')}`.toLocaleLowerCase().includes(query)) : items.value
})

function displayTags(image: DockerImage): string[] {
  return image.repo_tags.length ? image.repo_tags : ['<none>:<none>']
}

async function load() {
  const isCurrent = captureScope()
  const hostId = docker.selectedHostId
  items.value = []
  loading.value = false
  if (hostId === null || !auth.permissions.includes('docker:environment:read')) return
  loading.value = true
  try {
    const result = await imageApi.list(hostId)
    if (isCurrent()) items.value = result.items
  } catch (error) {
    if (isCurrent()) message.error(errorText(error, t('network.error')))
  } finally {
    if (isCurrent()) loading.value = false
  }
}

async function remove(image: DockerImage) {
  const hostId = docker.selectedHostId
  if (hostId === null || busy[image.id]) return
  busy[image.id] = true
  try {
    await imageApi.remove(hostId, image.id, force.value)
    message.success(t('docker.image.deleted'))
    void load()
  } catch (error) {
    message.error(errorText(error, t('network.error')))
  } finally {
    busy[image.id] = false
  }
}

function openPrune() {
  pruneAll.value = false
  pruneOpen.value = true
}

async function prune() {
  const hostId = docker.selectedHostId
  if (hostId === null) return
  pruning.value = true
  try {
    const result = await imageApi.prune(hostId, pruneAll.value)
    message.success(t('docker.image.pruned', { count: result.deleted.length, size: formatBytes(result.space_reclaimed) }))
    pruneOpen.value = false
    void load()
  } catch (error) {
    message.error(errorText(error, t('network.error')))
  } finally {
    pruning.value = false
  }
}

watch(() => docker.selectedHostId, () => { void load() }, { immediate: true })
</script>

<style scoped lang="scss">
.danger { color: var(--ant-color-error, #ff4d4f); }
.disabled { opacity: 0.5; pointer-events: none; }
</style>
