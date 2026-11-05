<template>
  <a-card>
    <PageHeader :title="t('assets.resource.instance.title')" />
    <AssetFilters class="u-mb-16" :controller="controller" @change="reload" />
    <a-alert v-if="errorMessage" type="error" :message="errorMessage" show-icon class="u-mb-16" />
    <a-table :columns="columns" :data-source="items" :loading="controller.loading.value" :pagination="false" row-key="asset_key" :scroll="{ x: 'max-content' }">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'native_id'"><RouterLink :to="{ name: 'assets.instances.detail', params: { id: record.asset_key } }" @click.stop>{{ record.native_id }}</RouterLink></template>
        <template v-else-if="column.key === 'account'">{{ record.cloud_account_name || record.cloud_account_id }}</template>
        <template v-else-if="column.key === 'location'">{{ locationLabel(record, t) }}</template>
        <template v-else-if="column.key === 'private_ip'">{{ observedAddresses(record.network_interfaces, 'private') || record.private_ip || '—' }}</template>
        <template v-else-if="column.key === 'public_ip'">{{ observedAddresses(record.network_interfaces, 'public') || record.public_ip || '—' }}</template>
        <template v-else-if="column.key === 'vpc'">{{ observedNetworks(record.network_interfaces) || record.vpc_id || '—' }}</template>
        <template v-else-if="column.key === 'tags'">{{ formatTags(record.tags) }}</template>
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
import type { InstanceSummary } from '@/types/assets'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import AssetFilters from '../components/AssetFilters.vue'
import { useResourceList, scalarFact, locationLabel, formatTags, observedAddresses, observedNetworks } from '../components/use-asset-list'

const { t } = useI18n()
const api = inject<AssetsResourceApi>('assetsResourceApi')!
const { controller, items, total, errorMessage, reload, onPageChange } = useResourceList<InstanceSummary>('instance', params => api.listInstances(params), ['state', 'vpc_id', 'ip', 'tag_key', 'tag_value'])
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

  { key: 'type', title: t('assets.resource.instance.type'), dataIndex: 'instance_type' },
  { key: 'state', title: t('assets.resource.instance.state'), dataIndex: 'state' },
  { key: 'private_ip', title: t('assets.resource.instance.observed_private_ips'), dataIndex: 'private_ip' },
  { key: 'public_ip', title: t('assets.resource.instance.observed_public_ips'), dataIndex: 'public_ip' },
  { key: 'vpc', title: t('assets.resource.instance.observed_networks'), dataIndex: 'vpc_id' },
  { key: 'tags', title: t('assets.resource.common.tags') },
  { key: 'last_seen_at', title: t('assets.resource.common.last_seen_at'), dataIndex: 'last_seen_at' },
  { key: 'deleted', title: '' },
])
</script>
