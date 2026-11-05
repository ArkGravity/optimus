<template>
  <a-card>
    <PageHeader :title="t('assets.resource.database.title')" />
    <AssetFilters class="u-mb-16" :controller="controller" @change="reload" />
    <a-alert v-if="errorMessage" type="error" :message="errorMessage" show-icon class="u-mb-16" />
    <a-table :columns="columns" :data-source="items" :loading="controller.loading.value" :pagination="false" row-key="asset_key" :scroll="{ x: 'max-content' }">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'native_id'"><RouterLink :to="{ name: 'assets.databases.detail', params: { id: record.asset_key } }" @click.stop>{{ record.native_id }}</RouterLink></template>
        <template v-else-if="column.key === 'account'">{{ record.cloud_account_name || record.cloud_account_id }}</template>
        <template v-else-if="column.key === 'location'">{{ locationLabel(record, t) }}</template>
        <template v-else-if="column.key === 'endpoint'">{{ formatEndpoints(record.endpoints) || record.endpoint || '—' }}</template>
        <template v-else-if="column.key === 'engine'">{{ record.engine }} {{ record.engine_version }}</template>
        <template v-else-if="column.key === 'multi_az' || column.key === 'publicly_accessible'">{{ booleanFact(record[column.key], t) }}</template>
        <template v-else-if="column.key === 'storage_gb'">{{ record.storage_gb ?? t('assets.detail.not_applicable') }}</template>
        <template v-else-if="column.key === 'deleted'"><a-tag v-if="record.deleted" color="red">{{ t('assets.resource.common.deleted_badge') }}</a-tag></template>
        <template v-else>{{ scalarFact(record[column.dataIndex], t) }}</template>
      </template>
    </a-table>
    <a-pagination class="u-mt-16" :current="controller.page.value" :page-size="controller.pageSize.value" :total="total" show-size-changer @change="onPageChange" />
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject } from 'vue'
import { RouterLink } from 'vue-router'
import type { AssetsResourceApi } from '@/api/assets/resource'
import type { DatabaseSummary } from '@/types/assets'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import AssetFilters from '../components/AssetFilters.vue'
import { useResourceList, scalarFact, locationLabel, booleanFact, formatEndpoints } from '../components/use-asset-list'

const { t } = useI18n()
const api = inject<AssetsResourceApi>('assetsResourceApi')!
const { controller, items, total, errorMessage, reload, onPageChange } = useResourceList<DatabaseSummary>('database', params => api.listDatabases(params), ['engine', 'status'])
const columns = computed(() => [
  { key: 'provider', title: t('assets.filters.provider'), dataIndex: 'provider' },
  { key: 'account', title: t('assets.account.title') },
  { key: 'project_id', title: t('assets.detail.fields.project_id'), dataIndex: 'project_id' },
  { key: 'location', title: t('assets.detail.fields.location') },
  { key: 'region', title: t('assets.filters.region'), dataIndex: 'region' },
  { key: 'zone', title: t('assets.filters.zone'), dataIndex: 'zone' },
  { key: 'native_id', title: t('assets.detail.fields.native_id') },
  { key: 'name', title: t('assets.detail.fields.name'), dataIndex: 'name' },
  { key: 'native_status', title: t('assets.detail.fields.native_status'), dataIndex: 'native_status' },

  { key: 'product', title: t('assets.detail.fields.product'), dataIndex: 'product' },
  { key: 'engine', title: t('assets.resource.database.engine') },
  { key: 'class', title: t('assets.resource.database.class'), dataIndex: 'instance_class' },
  { key: 'status', title: t('assets.resource.database.status'), dataIndex: 'status' },
  { key: 'endpoint', title: t('assets.resource.database.observed_endpoints'), dataIndex: 'endpoint' },
  { key: 'port', title: t('assets.resource.database.port'), dataIndex: 'port' },
  { key: 'multi_az', title: t('assets.resource.database.multi_az') },
  { key: 'publicly_accessible', title: t('assets.resource.database.publicly_accessible') },
  { key: 'storage_gb', title: t('assets.resource.database.storage_gb') },
  { key: 'last_seen_at', title: t('assets.resource.common.last_seen_at'), dataIndex: 'last_seen_at' },
  { key: 'deleted', title: '' },
])
</script>
