<template>
  <a-card>
    <PageHeader :title="t('assets.resource.firewall.title')" />
    <p>{{ t('assets.detail.firewall_inventory_notice') }}</p>
    <AssetFilters class="u-mb-16" :controller="controller" @change="reload" />
    <a-alert v-if="errorMessage" type="error" :message="errorMessage" show-icon class="u-mb-16" />
    <a-table :columns="columns" :data-source="items" :loading="controller.loading.value" :pagination="false" row-key="asset_key" :scroll="{ x: 'max-content' }">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'native_id'"><RouterLink :to="{ name: 'assets.firewalls.detail', params: { id: record.asset_key } }" @click.stop>{{ record.native_id }}</RouterLink></template>
        <template v-else-if="column.key === 'account'">{{ record.cloud_account_name || record.cloud_account_id }}</template>
        <template v-else-if="column.key === 'location'">{{ locationLabel(record, t) }}</template>
        <template v-else-if="column.key === 'priority'">{{ record.native_details.priority ?? t('assets.detail.not_applicable') }}</template>
        <template v-else-if="column.key === 'disabled'">{{ booleanFact(record.native_details.disabled, t) }}</template>
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
import type { FirewallSummary } from '@/types/assets'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import AssetFilters from '../components/AssetFilters.vue'
import { useResourceList, scalarFact, locationLabel, formatTags, booleanFact } from '../components/use-asset-list'

const { t } = useI18n()
const api = inject<AssetsResourceApi>('assetsResourceApi')!
const { controller, items, total, errorMessage, reload, onPageChange } = useResourceList<FirewallSummary>('firewall', params => api.listFirewalls(params), ['network_ref'])
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

  { key: 'network_ref', title: t('assets.filters.network_ref'), dataIndex: 'network_ref' },
  { key: 'description', title: t('assets.resource.security_group.description'), dataIndex: 'description' },
  { key: 'group_name', title: t('assets.resource.security_group.group_name'), dataIndex: 'group_name' },
  { key: 'owner_id', title: t('assets.resource.security_group.owner_id'), dataIndex: 'owner_id' },
  { key: 'inbound_rule_count', title: t('assets.resource.firewall.inbound_entries'), dataIndex: 'inbound_rule_count' },
  { key: 'outbound_rule_count', title: t('assets.resource.firewall.outbound_entries'), dataIndex: 'outbound_rule_count' },
  { key: 'priority', title: t('assets.firewall_rules.priority') },
  { key: 'disabled', title: t('assets.firewall_rules.disabled') },
  { key: 'tags', title: t('assets.resource.common.tags') },
  { key: 'last_seen_at', title: t('assets.resource.common.last_seen_at'), dataIndex: 'last_seen_at' },
  { key: 'deleted', title: '' },
])
</script>
