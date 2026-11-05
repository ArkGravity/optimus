<template>
  <a-card>
    <PageHeader :title="t('assets.sync.title')" />

    <div class="filter-row u-mb-16">
      <a-select data-testid="provider" :value="controller.filters.value.provider" :options="providerOptions" allow-clear :placeholder="t('assets.filters.provider')" :aria-label="t('assets.filters.provider')" @change="onProviderChange" />
      <a-select v-if="controller.canReadAccounts.value" data-testid="account" :value="controller.filters.value.account_id" :options="controller.accountOptions.value" :loading="controller.loadingAccounts.value" allow-clear show-search option-filter-prop="label" :placeholder="t('assets.account.title')" :aria-label="t('assets.account.title')" @change="onAccountChange" />
      <a-input-number v-else data-testid="account-id" :value="controller.filters.value.account_id" :min="1" :precision="0" :placeholder="t('assets.filters.account_id')" :aria-label="t('assets.filters.account_id')" @change="onAccountChange" />
      <a-select data-testid="location-type" :value="filters.location_type" :options="locationOptions" allow-clear :placeholder="t('assets.detail.fields.location_type')" :aria-label="t('assets.detail.fields.location_type')" @change="onLocationTypeChange" />
      <a-input data-testid="location" :value="filters.location" :disabled="filters.location_type === 'global'" allow-clear :placeholder="t('assets.detail.fields.location')" :aria-label="t('assets.detail.fields.location')" @change="onLocationChange" />
      <a-select data-testid="resource-type" :value="filters.resource_type" :options="resourceTypeOptions" allow-clear :placeholder="t('assets.sync.filter.resource_type')" :aria-label="t('assets.sync.filter.resource_type')" @change="onResourceTypeChange" />
      <a-select data-testid="status" :value="filters.status" :options="statusOptions" allow-clear :placeholder="t('assets.sync.filter.status')" :aria-label="t('assets.sync.filter.status')" @change="onStatusChange" />
      <a-date-picker :value="startedAfterInput" show-time allow-clear :placeholder="t('assets.sync.filter.started_after')" @change="onStartedAfterChange" />
    </div>
    <a-alert v-if="errorMessage" type="error" :message="errorMessage" show-icon class="u-mb-16" />

    <a-table
      :columns="columns"
      :data-source="items"
      :loading="controller.loading.value"
      :pagination="false"
      row-key="id"
      :scroll="{ x: 'max-content' }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'account'">{{ record.cloud_account_name || record.cloud_account_id }}</template>
        <template v-else-if="column.key === 'location'">{{ locationLabel(record, t) }}</template>
        <template v-else-if="column.key === 'started_at'">{{ formatTime(record.started_at) }}</template>
        <template v-else-if="column.key === 'finished_at'">{{ record.finished_at ? formatTime(record.finished_at) : '—' }}</template>
        <template v-else-if="column.key === 'duration'">{{ formatDuration(record) }}</template>
        <template v-else-if="column.key === 'resource_type'">{{ t(`assets.sync.resource_types.${record.resource_type}`) }}</template>
        <template v-else-if="column.key === 'status'">
          <a-tag :color="statusTagColor(record.status)">{{ t(`assets.sync.run_status.${record.status}`) }}</a-tag>
        </template>
        <template v-else-if="column.key === 'trigger'">{{ t(`assets.sync.triggers.${record.trigger}`) }}</template>
        <template v-else-if="column.key === 'error'">
          <a-tooltip v-if="record.error" :title="record.error">
            <button type="button" class="error-code" :aria-label="t('assets.sync.error')">
              {{ record.error_code ?? '—' }}
            </button>
          </a-tooltip>
          <template v-else>—</template>
        </template>
        <template v-else>{{ record[column.dataIndex] ?? '—' }}</template>
      </template>
    </a-table>

    <a-pagination
      class="u-mt-16"
      :current="controller.page.value"
      :page-size="controller.pageSize.value"
      :total="total"
      show-size-changer
      @change="onPageChange"
    />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch } from 'vue'
import dayjs from 'dayjs'
import type { AssetsSyncApi, SyncRunListParams } from '@/api/assets/sync'
import type { AssetsAccountApi } from '@/api/assets/account'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import { usePermission } from '@/hooks/usePermission'
import { useAssetFilters } from '@/hooks/use-asset-filters'
import { usePageState } from '@/hooks/usePageState'
import type { LocationType, Provider, SyncRunResourceType, SyncRunStatus, SyncRunSummary } from '@/types/assets'
import { useListData, locationLabel } from '../components/use-asset-list'

const { t } = useI18n()
const { has } = usePermission()
const api = inject<AssetsSyncApi>('assetsSyncApi')!
const controller = useAssetFilters({ family: 'instance', canReadAccounts: () => has('assets:account:read'), accountApi: inject<AssetsAccountApi>('assetsAccountApi') })
type SyncFilters = Omit<SyncRunListParams, 'provider' | 'account_id' | 'page' | 'size'>
const filters = ref<SyncFilters>({})
usePageState({ filters }, 'sync-filters')
const startedAfterInput = computed(() => filters.value.started_after ? dayjs(filters.value.started_after) : null)
watch(filters, () => { controller.page.value = 1; controller.invalidateRequests() }, { deep: true, flush: 'sync' })
const { items, total, errorMessage, reload, onPageChange } = useListData<SyncRunSummary>(controller, () => {
  const params: SyncRunListParams = { ...filters.value, page: controller.page.value, size: controller.pageSize.value }
  const { provider, account_id } = controller.filters.value
  if (provider) params.provider = provider
  if (account_id !== undefined) params.account_id = account_id
  return api.listRuns(params)
})
const providerOptions = [{ value: 'aws', label: 'AWS' }, { value: 'aliyun', label: 'Aliyun' }, { value: 'gcp', label: 'GCP' }]
const locationOptions = computed(() => (['global', 'region', 'zone'] as const).map(value => ({ value, label: t(`assets.filters.location.${value}`) })))
const resourceTypeOptions = computed(() => (['planning', 'instance', 'network', 'subnet', 'database', 'security_group', 'firewall'] as const).map(value => ({ value, label: t(`assets.sync.resource_types.${value}`) })))
const statusOptions = computed(() => (['running', 'success', 'failed', 'skipped'] as const).map(value => ({ value, label: t(`assets.sync.run_status.${value}`) })))
const statusColor: Record<SyncRunStatus, string> = { running: 'blue', success: 'green', failed: 'red', skipped: 'gold' }
const columns = computed(() => [
  { key: 'started_at', title: t('assets.sync.started_at') },
  { key: 'finished_at', title: t('assets.sync.finished_at') },
  { key: 'duration', title: t('assets.sync.duration') },
  { key: 'provider', title: t('assets.filters.provider'), dataIndex: 'provider' },
  { key: 'account', title: t('assets.account.title') },
  { key: 'location', title: t('assets.detail.fields.location') },
  { key: 'resource_type', title: t('assets.sync.resource_type') },
  { key: 'status', title: t('assets.sync.status') },
  { key: 'items_seen', title: t('assets.sync.items_seen'), dataIndex: 'items_seen' },
  { key: 'items_softdeleted', title: t('assets.sync.items_softdeleted'), dataIndex: 'items_softdeleted' },
  { key: 'trigger', title: t('assets.sync.trigger') },
  { key: 'error', title: t('assets.sync.error') },
])
function updateFilters(patch: Partial<SyncFilters>) {
  const next = { ...filters.value, ...patch }
  for (const key of Object.keys(next) as Array<keyof SyncFilters>) if (next[key] === undefined || next[key] === '') delete next[key]
  filters.value = next
  void reload()
}
function resetScope() { const next = { ...filters.value }; delete next.location_type; delete next.location; filters.value = next }
function onProviderChange(provider: Provider | undefined) { controller.setFilters({ provider }); resetScope(); void reload() }
function onAccountChange(value: number | string | null | undefined) {
  const id = value == null || value === '' ? undefined : Number(value)
  controller.setFilters({ account_id: id !== undefined && Number.isSafeInteger(id) && id > 0 ? id : undefined })
  resetScope(); void reload()
}
function onResourceTypeChange(value: SyncRunResourceType | undefined) { updateFilters({ resource_type: value }) }
function onStatusChange(value: SyncRunStatus | undefined) { updateFilters({ status: value }) }
function onLocationTypeChange(value: LocationType | undefined) { updateFilters({ location_type: value, location: undefined }) }
function onLocationChange(event: Event) { updateFilters({ location: (event.target as HTMLInputElement).value.trim() || undefined }) }
function onStartedAfterChange(value: { toDate?: () => Date } | null) {
  const date = value?.toDate?.()
  updateFilters({ started_after: date && Number.isFinite(date.getTime()) ? date.toISOString() : undefined })
}

function formatTime(value: string) {
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : '—'
}
function statusTagColor(value: SyncRunStatus) { return statusColor[value] }
function formatDuration(run: SyncRunSummary) {
  if (!run.finished_at) return '—'
  const milliseconds = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
  return Number.isFinite(milliseconds) && milliseconds >= 0 ? `${(milliseconds / 1000).toFixed(1)}s` : '—'
}

</script>

<style scoped lang="scss">
.filter-row { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
.filter-row > :deep(.ant-select), .filter-row > :deep(.ant-input-affix-wrapper), .filter-row > :deep(.ant-input-number) { width: 180px; }
.error-code {
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  cursor: help;
  text-decoration: underline dotted;
}
</style>
