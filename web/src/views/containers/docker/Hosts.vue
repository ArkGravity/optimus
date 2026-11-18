<template>
  <a-card class="resource-card" :title="$t('docker.sections.hosts')">
    <a-space class="resource-toolbar" wrap>
      <a-input-search
        v-model:value="search"
        allow-clear
        style="width: 260px"
        :placeholder="$t('docker.host.search')"
        @search="onSearch"
      />
      <a-button :loading="table.loading.value" @click="reload">
        <template #icon><ReloadOutlined /></template>
        {{ $t('common.refresh') }}
      </a-button>
      <a-button v-permission="WRITE" type="primary" class="create-host" @click="openCreate">
        <template #icon><PlusOutlined /></template>
        {{ $t('docker.host.create') }}
      </a-button>
    </a-space>

    <a-table
      :columns="columns"
      :data-source="table.items.value"
      :loading="table.loading.value"
      :pagination="false"
      :scroll="{ x: 'max-content' }"
      row-key="id"
      size="small"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'address'">{{ record.host }}:{{ record.port }}</template>
        <template v-else-if="column.key === 'auth'">
          {{ record.auth_type === 'password' ? $t('docker.host.auth_password') : (record.ssh_key_name || $t('docker.host.auth_ssh_key')) }}
        </template>
        <template v-else-if="column.key === 'status'">
          <a-tooltip :title="record.last_check_error || undefined">
            <a-badge :status="statusBadge(record.last_check_ok)" :text="statusLabel(record.last_check_ok)" />
          </a-tooltip>
        </template>
        <template v-else-if="column.key === 'last_checked_at'">{{ formatTime(record.last_checked_at) }}</template>
        <template v-else-if="column.key === 'created_at'">{{ formatTime(record.created_at) }}</template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a v-permission="WRITE" class="edit-host" @click="openEdit(record)">{{ $t('common.edit') }}</a>
            <a class="test-host" :class="{ disabled: testing[record.id] }" @click="test(record)">{{ $t('docker.host.test') }}</a>
            <a-popconfirm :title="$t('docker.host.confirm_delete', { name: record.name })" @confirm="remove(record)">
              <a v-permission="WRITE" class="delete-host danger">{{ $t('common.delete') }}</a>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <a-pagination
      class="u-mt-16"
      :current="table.page.value"
      :page-size="table.pageSize.value"
      :total="table.total.value"
      show-size-changer
      @change="table.setPage"
      @show-size-change="(_: number, size: number) => table.setPageSize(size)"
    />

    <HostForm v-model:open="formOpen" :initial="editing" @saved="reload" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { useI18n } from '@/hooks/useI18n'
import { usePageState } from '@/hooks/usePageState'
import { useTable } from '@/hooks/useTable'
import { useDockerStore } from '@/stores/docker'
import HostForm from './HostForm.vue'
import { errorText, formatTime } from './scope'
import type { DockerHostApi } from '@/api/docker/host'
import type { DockerHost } from '@/types/docker'

const WRITE = 'docker:environment:write'
const { t } = useI18n()
const hostApi = inject<DockerHostApi>('dockerHostApi')!
const docker = useDockerStore()
const search = ref('')
usePageState({ search })

const table = useTable<DockerHost, { search?: string }>({
  fetcher: async ({ page, pageSize, filters }) => {
    const result = await hostApi.list({ page, page_size: pageSize, search: filters?.search || undefined })
    return { items: result.items, total: result.total }
  },
})

const columns = computed(() => [
  { key: 'name', title: t('docker.host.name'), dataIndex: 'name' },
  { key: 'address', title: t('docker.host.address') },
  { key: 'auth', title: t('docker.host.auth') },
  { key: 'username', title: t('docker.host.username'), dataIndex: 'username' },
  { key: 'socket_path', title: t('docker.host.socket_path'), dataIndex: 'socket_path' },
  { key: 'status', title: t('docker.host.status') },
  { key: 'last_checked_at', title: t('docker.host.last_checked_at') },
  { key: 'created_at', title: t('common.created_at') },
  { key: 'actions', title: t('common.actions') },
])

const formOpen = ref(false)
const editing = ref<DockerHost | null>(null)
const testing = reactive<Record<number, boolean>>({})

function statusBadge(ok?: boolean) {
  return ok === true ? 'success' : ok === false ? 'error' : 'default'
}
function statusLabel(ok?: boolean) {
  return ok === true ? t('docker.host.reachable') : ok === false ? t('docker.host.unreachable') : t('docker.host.unknown')
}

function onSearch(value: string) {
  void table.setFilters({ search: value.trim() || undefined }).catch(error => message.error(errorText(error, t('network.error'))))
}

function openCreate() {
  editing.value = null
  formOpen.value = true
}

async function openEdit(host: DockerHost) {
  try {
    editing.value = await hostApi.get(host.id)
    formOpen.value = true
  } catch (error) {
    message.error(errorText(error, t('network.error')))
  }
}

async function test(host: DockerHost) {
  if (testing[host.id]) return
  testing[host.id] = true
  try {
    const result = await hostApi.test(host.id)
    if (result.ok) message.success(t('docker.host.test_ok', { version: result.server_version || '-' }))
    else message.error(result.message || t('docker.host.test_failed'))
    reload()
  } catch (error) {
    message.error(errorText(error, t('network.error')))
  } finally {
    testing[host.id] = false
  }
}

async function remove(host: DockerHost) {
  try {
    await hostApi.remove(host.id)
    docker.clearHost(host.id)
    message.success(t('docker.host.deleted'))
    reload()
  } catch (error) {
    message.error(errorText(error, t('network.error')))
  }
}

function reload() {
  void table.reload().catch(error => message.error(errorText(error, t('network.error'))))
}

onMounted(reload)
</script>

<style scoped lang="scss">
.danger { color: var(--ant-color-error, #ff4d4f); }
.disabled { opacity: 0.5; pointer-events: none; }
</style>
