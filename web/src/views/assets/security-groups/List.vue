<template>
  <a-card>
    <PageHeader :title="t('assets.resource.security_group.title')" />
    <div class="filter-row u-mb-16">
      <a-input-search v-model:value="searchInput" allow-clear :placeholder="t('assets.resource.security_group.group_name')" style="width: 220px" @search="onSearch" @change="onSearchInputChange" />
      <a-select v-if="canReadAccounts" v-model:value="accountID" data-testid="account" allow-clear show-search option-filter-prop="label" :placeholder="t('assets.account.title')" :options="accountOptions" :loading="loadingAccounts" style="width: 150px" @change="onAccountChange" />
      <a-input-number v-else v-model:value="accountID" data-testid="account-id" :min="1" :precision="0" :placeholder="t('assets.account.title')" style="width: 150px" @change="onAccountChange" />
      <a-input v-model:value="regionInput" allow-clear :placeholder="t('assets.account.regions')" style="width: 150px" @press-enter="onRegionSubmit" @change="onRegionInputChange" />
      <a-input v-model:value="vpcInput" allow-clear :placeholder="t('assets.resource.vpc.vpc_id')" style="width: 180px" @press-enter="onVPCSubmit" @change="onVPCChange" />
      <a-checkbox :checked="includeDeleted" @change="onIncludeDeletedChange">{{ t('assets.resource.common.include_deleted') }}</a-checkbox>
    </div>
    <a-table :columns="columns" :data-source="table.items.value" :loading="table.loading.value" :pagination="false" :custom-row="securityGroupRow" :scroll="{ x: 1400 }" row-key="id">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'group_id'"><a-button type="link" @click.stop="openDetails(record)">{{ record.group_id }}</a-button></template>
        <template v-else-if="column.key === 'deleted'"><a-tag v-if="record.deleted" color="red">{{ t('assets.resource.common.deleted_badge') }}</a-tag></template>
        <template v-else>{{ record[column.dataIndex] ?? '—' }}</template>
      </template>
    </a-table>
    <a-pagination class="u-mt-16" :current="table.page.value" :page-size="table.pageSize.value" :total="table.total.value" show-size-changer @change="onPageChange" @show-size-change="onPageSizeChange" />

    <a-drawer :open="detailsOpen" :width="1000" :title="t('assets.resource.security_group.details')" @close="closeDetails">
      <a-spin :spinning="detailsLoading">
        <template v-if="detail">
          <a-descriptions :column="2" bordered size="small" class="u-mb-16">
            <a-descriptions-item :label="t('assets.account.title')">{{ detail.cloud_account_name }}</a-descriptions-item>
            <a-descriptions-item :label="t('assets.account.regions')">{{ detail.region }}</a-descriptions-item>
            <a-descriptions-item :label="t('assets.resource.security_group.group_id')">{{ detail.group_id }}</a-descriptions-item>
            <a-descriptions-item :label="t('assets.resource.security_group.group_name')">{{ detail.group_name }}</a-descriptions-item>
            <a-descriptions-item :label="t('assets.resource.vpc.vpc_id')">{{ detail.vpc_id || '—' }}</a-descriptions-item>
            <a-descriptions-item :label="t('assets.resource.security_group.owner_id')">{{ detail.owner_id || '—' }}</a-descriptions-item>
            <a-descriptions-item :label="t('assets.resource.security_group.description')">{{ detail.description || '—' }}</a-descriptions-item>
            <a-descriptions-item :label="t('assets.resource.common.tags')">{{ Object.entries(detail.tags).map(([key, value]) => `${key}=${value}`).join(', ') || '—' }}</a-descriptions-item>
          </a-descriptions>
          <a-tag v-if="detail.deleted" color="red">{{ t('assets.resource.common.deleted_badge') }}</a-tag>
          <a-tabs>
            <a-tab-pane v-for="tab in ruleTabs" :key="tab.key" :tab="tab.title">
              <a-table :columns="ruleColumns" :data-source="tab.rules.map((rule, index) => ({ ...rule, key: index }))" row-key="key" :scroll="{ x: 900 }" :pagination="{ pageSize: 20 }" size="small">
                <template #bodyCell="{ column, record }">
                  <template v-if="column.key === 'protocol'">{{ formatRuleProtocol(record, t('assets.resource.security_group.all')) }}</template>
                  <template v-else-if="column.key === 'ports'">{{ formatRulePorts(record, t('assets.resource.security_group.all')) }}</template>
                  <template v-else-if="column.key === 'target_type'">{{ t(`assets.resource.security_group.target_types.${record.target_type}`) }}</template>
                  <template v-else>{{ record[column.dataIndex] || '—' }}</template>
                </template>
              </a-table>
            </a-tab-pane>
          </a-tabs>
        </template>
      </a-spin>
    </a-drawer>
  </a-card>
</template>

<script setup lang="ts">
import { usePageState } from '@/hooks/usePageState'
import { computed, inject, onBeforeUnmount, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import type { AssetsAccountApi } from '@/api/assets/account'
import type { AssetsResourceApi, SecurityGroupListParams } from '@/api/assets/resource'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import { usePermission } from '@/hooks/usePermission'
import { useTable } from '@/hooks/useTable'
import { formatRulePorts, formatRuleProtocol } from './rules'
import type { SecurityGroupDetail, SecurityGroupSummary } from '@/types/assets'
import { isBizError } from '@/utils/http-error'

const { t } = useI18n()
const { has } = usePermission()
const canReadAccounts = has('assets:account:read')
const api = inject<AssetsResourceApi>('assetsResourceApi')!
const accountApi = inject<AssetsAccountApi>('assetsAccountApi')!
const searchInput = ref('')
const accountID = ref<number>()
const regionInput = ref('')
const vpcInput = ref('')
const detailsOpen = ref(false)
const detailsLoading = ref(false)
const detail = ref<SecurityGroupDetail>()
let detailRequest = 0
const includeDeleted = ref(false)
const accountOptions = ref<Array<{ label: string; value: number }>>([])
const loadingAccounts = ref(false)
const columns = computed(() => [
  { key: 'account', title: t('assets.account.title'), dataIndex: 'cloud_account_name' },
  { key: 'region', title: t('assets.account.regions'), dataIndex: 'region' },
  { key: 'group_id', title: t('assets.resource.security_group.group_id') },
  { key: 'group_name', title: t('assets.resource.security_group.group_name'), dataIndex: 'group_name' },
  { key: 'name', title: t('assets.resource.instance.name'), dataIndex: 'name' },
  { key: 'vpc_id', title: t('assets.resource.vpc.vpc_id'), dataIndex: 'vpc_id' },
  { key: 'inbound', title: t('assets.resource.security_group.inbound'), dataIndex: 'inbound_rule_count' },
  { key: 'outbound', title: t('assets.resource.security_group.outbound'), dataIndex: 'outbound_rule_count' },
  { key: 'last_seen_at', title: t('assets.resource.common.last_seen_at'), dataIndex: 'last_seen_at' },
  { key: 'deleted', title: '' },
])
const ruleColumns = computed(() => [
  { key: 'protocol', title: t('assets.resource.security_group.protocol') },
  { key: 'ports', title: t('assets.resource.security_group.ports') },
  { key: 'target_type', title: t('assets.resource.security_group.target_type') },
  { key: 'target', title: t('assets.resource.security_group.source') + ' / ' + t('assets.resource.security_group.destination'), dataIndex: 'target' },
  { key: 'owner', title: t('assets.resource.security_group.owner_id'), dataIndex: 'owner_id' },
  { key: 'vpc', title: t('assets.resource.vpc.vpc_id'), dataIndex: 'vpc_id' },
  { key: 'description', title: t('assets.resource.security_group.description'), dataIndex: 'description' },
])
const ruleTabs = computed(() => [
  { key: 'inbound', title: t('assets.resource.security_group.inbound'), rules: detail.value?.inbound_rules ?? [] },
  { key: 'outbound', title: t('assets.resource.security_group.outbound'), rules: detail.value?.outbound_rules ?? [] },
])
const table = useTable<SecurityGroupSummary, SecurityGroupListParams>({ fetcher: async ({ page, pageSize, filters }) => {
  const result = await api.listSecurityGroups({ page, size: pageSize, ...filters }); return { items: result.items, total: result.total }
} })
function showLoadError(error: unknown) { message.error(isBizError(error) ? error.message : t('network.error')) }
async function runTableAction(action: () => Promise<unknown>) { try { await action() } catch (error) { showLoadError(error) } }
function clean(value: string) { return value.trim() || undefined }
async function onSearch(value: string) { await runTableAction(() => table.setFilters({ q: clean(value) })) }
async function onSearchInputChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ q: undefined })) }
async function onAccountChange(value: number | null | undefined) { await runTableAction(() => table.setFilters({ account_id: value ?? undefined })) }
async function onRegionSubmit() { await runTableAction(() => table.setFilters({ region: clean(regionInput.value) })) }
async function onRegionInputChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ region: undefined })) }
async function onIncludeDeletedChange(event: { target?: { checked?: boolean } }) { includeDeleted.value = Boolean(event.target?.checked); await runTableAction(() => table.setFilters({ include_deleted: includeDeleted.value || undefined })) }
async function onPageChange(page: number) { await runTableAction(() => table.setPage(page)) }
async function onPageSizeChange(_: number, size: number) { await runTableAction(() => table.setPageSize(size)) }
async function onVPCSubmit() { await runTableAction(() => table.setFilters({ vpc_id: clean(vpcInput.value) })) }
async function onVPCChange(event: Event) { if ((event.target as HTMLInputElement | null)?.value === '') await runTableAction(() => table.setFilters({ vpc_id: undefined })) }
function closeDetails() { detailsOpen.value = false; detailRequest++; detail.value = undefined; detailsLoading.value = false }
async function openDetails(record: SecurityGroupSummary) {
  const request = ++detailRequest
  detail.value = undefined
  detailsOpen.value = true
  detailsLoading.value = true
  try {
    const result = await api.getSecurityGroup(record.id)
    if (request === detailRequest) detail.value = result
  } catch (error) { if (request === detailRequest) { closeDetails(); showLoadError(error) } }
  finally { if (request === detailRequest) detailsLoading.value = false }
}
function securityGroupRow(record: SecurityGroupSummary) {
  return { onClick: () => openDetails(record), tabindex: 0, role: 'button', onKeydown: (event: KeyboardEvent) => {
    if (event.target === event.currentTarget && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); void openDetails(record) }
  } }
}
onBeforeUnmount(closeDetails)
async function loadAccounts() {
  loadingAccounts.value = true
  try {
    const accounts: Array<{ id: number; name: string }> = []
    let page = 1
    let total = 0
    do {
      const result = await accountApi.list({ page, size: 100 })
      accounts.push(...result.items)
      total = result.total
      page += 1
      if (result.items.length === 0) break
    } while (accounts.length < total)
    accountOptions.value = accounts.map(account => ({ label: account.name, value: account.id }))
  } catch (error) { showLoadError(error) } finally { loadingAccounts.value = false }
}
onMounted(() => {
  if (canReadAccounts) void loadAccounts()
  void runTableAction(() => table.reload())
})
usePageState({ searchInput, accountID, regionInput, vpcInput, includeDeleted })

</script>

<style scoped lang="scss">
.filter-row { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
:deep(.ant-table-tbody > tr) { cursor: pointer; }
</style>
