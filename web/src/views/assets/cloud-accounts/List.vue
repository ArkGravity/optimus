<template>
  <a-card>
    <PageHeader :title="t('assets.account.title')">
      <a-button
        v-permission="'assets:account:write'"
        data-testid="create-account"
        type="primary"
        @click="openCreate"
      >
        {{ t('assets.account.actions.create') }}
      </a-button>
    </PageHeader>

    <div class="filter-row u-mb-16">
      <a-input-search
        v-model:value="searchInput"
        allow-clear
        :aria-label="t('assets.account.name')"
        :placeholder="t('assets.account.name')"
        style="width: 280px;"
        @search="onSearch"
        @change="onSearchInputChange"
      />
      <a-select
        v-model:value="providerFilter"
        data-testid="provider"
        allow-clear
        :aria-label="t('assets.account.provider')"
        :placeholder="t('assets.account.provider')"
        style="width: 140px;"
        :options="providerOptions"
        @change="onProviderChange"
      />
      <a-select
        v-model:value="enabledFilter"
        allow-clear
        :aria-label="t('assets.account.enabled')"
        :placeholder="t('assets.account.enabled')"
        style="width: 140px;"
        :options="enabledOptions"
        @change="onEnabledChange"
      />
    </div>

    <a-table
      :columns="columns"
      :data-source="table.items.value"
      :loading="table.loading.value"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'provider'">{{ providerNames[record.provider as Provider] }}</template>
        <template v-else-if="column.key === 'project_id' || column.key === 'external_account_id'">{{ record[column.key] || t('assets.detail.not_applicable') }}</template>
        <template v-else-if="column.key === 'enabled'">
          <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '✓' : '—' }}</a-tag>
        </template>
        <template v-else-if="column.key === 'last_sync_at'">
          {{ record.last_sync_at ? formatTime(record.last_sync_at) : '—' }}
        </template>
        <template v-else-if="column.key === 'last_sync_status'">
          <a-tag v-if="record.last_sync_status" :color="statusColor(record.last_sync_status)">
            {{ t(`assets.sync.run_status.${record.last_sync_status}`) }}
          </a-tag>
          <template v-else>—</template>
        </template>
        <template v-else-if="column.key === 'actions'">
          <a-space>
            <a-button
              v-permission="'assets:account:write'"
              :data-testid="`sync-${record.id}`"
              type="link"
              :disabled="store.syncInFlight[record.id]"
              :aria-label="t('assets.account.actions.sync_now')"
              @click="syncNow(record)"
            >{{ t('assets.account.actions.sync_now') }}</a-button>
            <a-button
              v-permission="'assets:account:write'"
              :data-testid="`edit-${record.id}`"
              type="link"
              @click="openEdit(record)"
            >{{ t('assets.account.actions.edit') }}</a-button>
            <a-popconfirm :title="t('assets.account.delete_confirm', { name: record.name })" @confirm="remove(record)">
              <a
                v-permission="'assets:account:delete'"
                :data-testid="`delete-${record.id}`"
                class="danger"
              >{{ t('assets.account.actions.delete') }}</a>
            </a-popconfirm>
          </a-space>
        </template>
        <template v-else>{{ record[column.dataIndex] }}</template>
      </template>
    </a-table>

    <a-pagination
      class="u-mt-16"
      :current="table.page.value"
      :page-size="table.pageSize.value"
      :total="table.total.value"
      show-size-changer
      @change="onPageChange"
    />

    <Form :open="formOpen" :editing="editing" @close="closeForm" @saved="onSaved" />
  </a-card>
</template>

<script setup lang="ts">
import { usePageState } from '@/hooks/usePageState'
import { computed, inject, onBeforeUnmount, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import type { AssetsAccountApi, AssetsAccountListParams } from '@/api/assets/account'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import { useAssetsStore } from '@/stores/assets'
import type { CloudAccountDetail, CloudAccountSummary, Provider } from '@/types/assets'
import { isBizError } from '@/utils/http-error'
import Form from './Form.vue'

const accountApi = inject<AssetsAccountApi>('assetsAccountApi')!
const { t } = useI18n()
const store = useAssetsStore()
const searchInput = ref('')
const enabledFilter = ref<boolean | undefined>()
const providerFilter = ref<Provider | undefined>()
const providerNames: Record<Provider, string> = { aws: 'AWS', aliyun: 'Aliyun', gcp: 'GCP' }
const providerOptions = Object.entries(providerNames).map(([value, label]) => ({ value, label }))
const formOpen = ref(false)
const editing = ref<CloudAccountDetail | null>(null)
let editGeneration = 0
let listGeneration = 0
let alive = true

const enabledOptions = computed(() => [
  { value: true, label: t('assets.account.enabled') },
  { value: false, label: t('system.users.status_disabled') },
])
// Account filters retain the existing workspace keys. Responses and request
// ownership belong only to this mounted list.
const table = {
  page: ref(1), pageSize: ref(20), filters: ref<AssetsAccountListParams>({}),
  items: ref<CloudAccountSummary[]>([]), total: ref(0), loading: ref(false),
  reload: loadAccounts, setFilters: setTableFilters, setPage: setTablePage, setPageSize: setTablePageSize,
}
usePageState({ page: table.page, pageSize: table.pageSize, filters: table.filters }, 'table')
async function loadAccounts() {
  const generation = ++listGeneration
  const owns = () => alive && generation === listGeneration
  table.loading.value = true
  table.items.value = []
  table.total.value = 0
  const filters = table.filters.value
  try {
    const result = await accountApi.list({ page: table.page.value, size: table.pageSize.value, q: filters.q || undefined, enabled: filters.enabled, provider: filters.provider })
    if (!owns()) return
    table.items.value = result.items
    table.total.value = result.total
  } catch (error) {
    if (owns()) throw error
  } finally {
    if (owns()) table.loading.value = false
  }
}
async function setTableFilters(patch: Partial<AssetsAccountListParams>) {
  table.filters.value = { ...table.filters.value, ...patch }
  table.page.value = 1
  await loadAccounts()
}
async function setTablePage(page: number) { table.page.value = page; await loadAccounts() }
async function setTablePageSize(size: number) { table.pageSize.value = size; table.page.value = 1; await loadAccounts() }
const columns = computed(() => [
  { key: 'name', title: t('assets.account.name'), dataIndex: 'name' },
  { key: 'provider', title: t('assets.account.provider'), dataIndex: 'provider' },
  { key: 'project_id', title: t('assets.account.project_id'), dataIndex: 'project_id' },
  { key: 'external_account_id', title: t('assets.account.external_account_id'), dataIndex: 'external_account_id' },
  { key: 'scope_revision', title: t('assets.account.scope_revision'), dataIndex: 'scope_revision' },
  { key: 'cloudkey_name', title: t('assets.account.cloudkey'), dataIndex: 'cloudkey_name' },
  { key: 'regions_count', title: t('assets.account.regions'), dataIndex: 'regions_count' },
  { key: 'enabled', title: t('assets.account.enabled') },
  { key: 'last_sync_at', title: t('assets.account.last_sync_at') },
  { key: 'last_sync_status', title: t('assets.account.last_sync_status') },
  { key: 'actions', title: '', width: 240 },
])

function showError(error: unknown) {
  message.error(isBizError(error) ? error.message : t('network.error'))
}
async function runTableAction(action: () => Promise<unknown>) {
  try {
    await action()
  } catch (error) {
    showError(error)
  }
}
async function reloadTable() { await runTableAction(() => table.reload()) }
async function onSearch(value: string) { await runTableAction(() => table.setFilters({ q: value || undefined })) }
async function onSearchInputChange(event: Event) {
  const target = event.target as HTMLInputElement | null
  if (target?.value === '') await runTableAction(() => table.setFilters({ q: undefined }))
}
async function onEnabledChange(value: boolean | undefined) {
  await runTableAction(() => table.setFilters({ enabled: value }))
}
async function onProviderChange(value: Provider | undefined) { await runTableAction(() => table.setFilters({ provider: value })) }
async function onPageChange(page: number, size = table.pageSize.value) {
  if (size !== table.pageSize.value) await runTableAction(() => table.setPageSize(size))
  else await runTableAction(() => table.setPage(page))
}
function openCreate() { editGeneration++; editing.value = null; formOpen.value = true }
function closeForm() { editGeneration++; formOpen.value = false; editing.value = null }
async function openEdit(account: CloudAccountSummary) {
  const generation = ++editGeneration
  try {
    const result = await accountApi.get(account.id)
    if (!alive || generation !== editGeneration) return
    editing.value = result
    formOpen.value = true
  } catch (error) {
    if (alive && generation === editGeneration) showError(error)
  }
}
async function onSaved() { closeForm(); await reloadTable() }

async function syncNow(account: CloudAccountSummary) {
  if (store.syncInFlight[account.id]) return
  store.markSyncStarted(account.id)
  window.setTimeout(() => store.clearSyncStarted(account.id), 30_000)
  try {
    await accountApi.triggerSync(account.id)
    message.success(t('assets.account.sync_queued'))
  } catch (error) {
    showError(error)
  }
}

async function remove(account: CloudAccountSummary) {
  try {
    const result = await accountApi.remove(account.id)
    message.success(t('assets.account.cascaded_resources', { count: result.cascaded_resources_count }))
    await table.reload()
  } catch (error) {
    showError(error)
  }
}
function statusColor(status: string) {
  return ({ success: 'green', failed: 'red', running: 'blue', skipped: 'gold' } as Record<string, string>)[status] ?? 'default'
}
function formatTime(value: string) { return new Date(value).toLocaleString() }

onMounted(reloadTable)
onBeforeUnmount(() => { alive = false; editGeneration++; listGeneration++; table.loading.value = false })
usePageState({ searchInput, enabledFilter, providerFilter })

</script>

<style scoped lang="scss">
.filter-row { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
.danger { color: var(--ant-color-error, #ff4d4f); }
</style>
