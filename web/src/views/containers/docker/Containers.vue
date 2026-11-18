<template>
  <a-card class="resource-card" :title="$t('docker.sections.containers')">
    <a-space class="resource-toolbar" wrap>
      <HostPicker />
      <a-input v-model:value="search" allow-clear style="width: 240px" :placeholder="$t('docker.container.search')" />
      <a-button :loading="loading" :disabled="docker.selectedHostId === null" @click="load">
        <template #icon><ReloadOutlined /></template>
        {{ $t('common.refresh') }}
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
        <template v-if="column.key === 'name'">
          <a class="open-detail" @click="openDetail(record)">{{ record.name || record.short_id }}</a>
        </template>
        <template v-else-if="column.key === 'short_id'"><code>{{ record.short_id }}</code></template>
        <template v-else-if="column.key === 'state'">
          <a-tag :color="stateColor(record.state)">{{ record.state }}</a-tag>
        </template>
        <template v-else-if="column.key === 'ports'">
          <div v-for="port in record.ports" :key="port">{{ port }}</div>
          <span v-if="!record.ports?.length">-</span>
        </template>
        <template v-else-if="column.key === 'created'">{{ formatTime(record.created) }}</template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a
              v-for="action in ACTIONS"
              :key="action"
              v-permission="WRITE"
              :class="['container-action', `container-${action}`, { disabled: busy[record.id] || !allowed(action, record.state) }]"
              @click="run(record, action)"
            >{{ $t(`docker.container.${action}`) }}</a>
            <a-popconfirm @confirm="remove(record)" @open-change="(open: boolean) => { if (open) force = record.state === 'running' }">
              <template #title>
                <div>{{ $t('docker.container.confirm_delete', { name: record.name || record.short_id }) }}</div>
                <a-checkbox v-model:checked="force">{{ $t('docker.container.force') }}</a-checkbox>
              </template>
              <a v-permission="WRITE" :class="['delete-container', 'danger', { disabled: busy[record.id] || record.state === 'removing' }]">{{ $t('common.delete') }}</a>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <a-drawer :open="detailOpen" :width="720" :title="detail?.name || $t('docker.container.detail')" @close="closeDetail">
      <a-spin :spinning="detailLoading">
        <a-descriptions v-if="detail" :column="1" bordered size="small">
          <a-descriptions-item :label="$t('docker.container.id')"><code>{{ detail.id }}</code></a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.name')">{{ detail.name }}</a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.image')">{{ detail.image }}</a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.state')">
            <a-tag :color="stateColor(detail.state.status)">{{ detail.state.status }}</a-tag>
            <span v-if="detail.state.health">{{ $t('docker.container.health') }}: {{ detail.state.health }}</span>
            <span v-if="!detail.state.running">{{ $t('docker.container.exit_code') }}: {{ detail.state.exit_code }}</span>
            <div v-if="detail.state.error" class="error-text">{{ detail.state.error }}</div>
          </a-descriptions-item>
          <a-descriptions-item :label="$t('common.created_at')">{{ formatTime(detail.created) }}</a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.started_at')">{{ formatTime(detail.state.started_at) }}</a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.finished_at')">{{ formatTime(detail.state.finished_at) }}</a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.command')"><code>{{ detail.command.join(' ') || '-' }}</code></a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.restart_policy')">
            {{ detail.restart_policy.name || 'no' }}
            <span v-if="detail.restart_policy.maximum_retry_count">({{ detail.restart_policy.maximum_retry_count }})</span>
            · {{ $t('docker.container.restart_count') }}: {{ detail.restart_count }}
          </a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.ports')">
            <div v-for="port in detail.ports" :key="port">{{ port }}</div>
            <span v-if="!detail.ports.length">-</span>
          </a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.networks')">
            <div v-for="network in detail.networks" :key="network.name">
              {{ network.name }}: {{ [network.ip_address, network.ipv6_address].filter(Boolean).join(', ') || '-' }}
            </div>
            <span v-if="!detail.networks.length">-</span>
          </a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.mounts')">
            <div v-for="mount in detail.mounts" :key="mount.destination">
              {{ mount.type }} {{ mount.name || mount.source }} → {{ mount.destination }} ({{ mount.rw ? 'rw' : 'ro' }})
            </div>
            <span v-if="!detail.mounts.length">-</span>
          </a-descriptions-item>
          <a-descriptions-item :label="$t('docker.container.env')">
            <a-collapse v-if="detail.env.length" ghost>
              <a-collapse-panel key="env" :header="$t('docker.container.env_count', { count: detail.env.length })">
                <pre class="env">{{ detail.env.join('\n') }}</pre>
              </a-collapse-panel>
            </a-collapse>
            <span v-else>-</span>
          </a-descriptions-item>
        </a-descriptions>
      </a-spin>
    </a-drawer>
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import { useI18n } from '@/hooks/useI18n'
import { usePageState } from '@/hooks/usePageState'
import { useAuthStore } from '@/stores/auth'
import { useDockerStore } from '@/stores/docker'
import HostPicker from './HostPicker.vue'
import { errorText, formatTime, useHostScope } from './scope'
import type { DockerContainerApi } from '@/api/docker/container'
import type { DockerContainer, DockerContainerAction, DockerContainerDetail } from '@/types/docker'

const WRITE = 'docker:environment:write'
const ACTIONS: readonly DockerContainerAction[] = ['start', 'stop', 'restart']
const { t } = useI18n()
const containerApi = inject<DockerContainerApi>('dockerContainerApi')!
const docker = useDockerStore()
const auth = useAuthStore()
const captureScope = useHostScope()
const search = ref('')
usePageState({ search })
const items = ref<DockerContainer[]>([])
const loading = ref(false)
const busy = reactive<Record<string, boolean>>({})
const force = ref(false)
const detailOpen = ref(false)
const detailLoading = ref(false)
const detail = ref<DockerContainerDetail | null>(null)

const columns = computed(() => [
  { key: 'name', title: t('docker.container.name') },
  { key: 'short_id', title: t('docker.container.id') },
  { key: 'image', title: t('docker.container.image'), dataIndex: 'image' },
  { key: 'status', title: t('docker.container.status'), dataIndex: 'status' },
  { key: 'state', title: t('docker.container.state') },
  { key: 'ports', title: t('docker.container.ports') },
  { key: 'created', title: t('common.created_at') },
  { key: 'actions', title: t('common.actions') },
])
const filtered = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return query ? items.value.filter(row => `${row.name} ${row.id} ${row.image}`.toLocaleLowerCase().includes(query)) : items.value
})

function allowed(action: DockerContainerAction, state: string): boolean {
  switch (action) {
    case 'start': return state === 'created' || state === 'exited'
    case 'stop': return state === 'running' || state === 'restarting' || state === 'paused'
    case 'restart': return state === 'running' || state === 'exited' || state === 'created'
    default: {
      const unreachable: never = action
      return unreachable
    }
  }
}

function stateColor(state: string): string {
  if (state === 'running') return 'green'
  if (state === 'paused' || state === 'restarting') return 'orange'
  if (state === 'dead') return 'red'
  return 'default'
}

async function load() {
  const isCurrent = captureScope()
  const hostId = docker.selectedHostId
  items.value = []
  loading.value = false
  if (hostId === null || !auth.permissions.includes('docker:environment:read')) return
  loading.value = true
  try {
    const result = await containerApi.list(hostId)
    if (isCurrent()) items.value = result.items
  } catch (error) {
    if (isCurrent()) message.error(errorText(error, t('network.error')))
  } finally {
    if (isCurrent()) loading.value = false
  }
}

async function run(row: DockerContainer, action: DockerContainerAction) {
  const hostId = docker.selectedHostId
  if (hostId === null || busy[row.id] || !allowed(action, row.state)) return
  busy[row.id] = true
  try {
    await containerApi.action(hostId, row.id, action)
    message.success(t(`docker.container.${action}_done`, { name: row.name }))
    void load()
  } catch (error) {
    message.error(errorText(error, t('network.error')))
  } finally {
    busy[row.id] = false
  }
}

async function remove(row: DockerContainer) {
  const hostId = docker.selectedHostId
  if (hostId === null || busy[row.id]) return
  busy[row.id] = true
  try {
    await containerApi.remove(hostId, row.id, force.value)
    message.success(t('docker.container.deleted', { name: row.name }))
    void load()
  } catch (error) {
    message.error(errorText(error, t('network.error')))
  } finally {
    busy[row.id] = false
  }
}

async function openDetail(row: DockerContainer) {
  const hostId = docker.selectedHostId
  if (hostId === null) return
  const isCurrent = captureScope('detail')
  detail.value = null
  detailOpen.value = true
  detailLoading.value = true
  try {
    const result = await containerApi.get(hostId, row.id)
    if (isCurrent()) detail.value = result
  } catch (error) {
    if (isCurrent()) {
      message.error(errorText(error, t('network.error')))
      detailOpen.value = false
    }
  } finally {
    if (isCurrent()) detailLoading.value = false
  }
}

function closeDetail() {
  captureScope('detail')
  detailOpen.value = false
  detailLoading.value = false
  detail.value = null
}

watch(() => docker.selectedHostId, () => {
  closeDetail()
  void load()
}, { immediate: true })
</script>

<style scoped lang="scss">
.danger { color: var(--ant-color-error, #ff4d4f); }
.disabled { opacity: 0.5; pointer-events: none; }
.error-text { color: var(--ant-color-error, #ff4d4f); }
.env { margin: 0; white-space: pre-wrap; word-break: break-all; font-size: 12px; }
</style>
