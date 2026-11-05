<template>
  <div class="asset-filters">
    <a-select
      data-testid="provider" :value="controller.filters.value.provider" :options="providers"
      allow-clear :placeholder="t('assets.filters.provider')" :aria-label="t('assets.filters.provider')"
      @change="setProvider"
    />
    <a-select
      v-if="controller.canReadAccounts.value" data-testid="account" :value="controller.filters.value.account_id"
      :options="controller.accountOptions.value" :loading="controller.loadingAccounts.value"
      allow-clear show-search option-filter-prop="label"
      :placeholder="t('assets.account.title')" :aria-label="t('assets.account.title')" @change="setAccount"
    />
    <a-input-number
      v-else data-testid="account-id" :value="controller.filters.value.account_id" :min="1" :precision="0"
      :placeholder="t('assets.filters.account_id')" :aria-label="t('assets.filters.account_id')" @change="setAccount"
    />
    <a-input
      v-for="field in commonFields" :key="field" :data-testid="field" :value="controller.filters.value[field]"
      allow-clear :placeholder="t(`assets.filters.${field}`)" :aria-label="t(`assets.filters.${field}`)"
      @change="setText(field, $event)"
    />
    <template v-if="controller.family === 'instance'">
      <a-select
        data-testid="state" :value="controller.filters.value.state" :options="stateOptions" allow-clear
        :placeholder="t('assets.resource.instance.state')" :aria-label="t('assets.resource.instance.state')"
        @change="setValue('state', $event)"
      />
      <a-input
        v-for="field in instanceFields" :key="field" :data-testid="field" :value="controller.filters.value[field]"
        allow-clear :placeholder="t(`assets.filters.${field}`)" :aria-label="t(`assets.filters.${field}`)"
        @change="setText(field, $event)"
      />
    </template>
    <template v-if="controller.family === 'database'">
      <a-input
        data-testid="engine" :value="controller.filters.value.engine" allow-clear
        :placeholder="t('assets.resource.database.engine')" :aria-label="t('assets.resource.database.engine')" @change="setText('engine', $event)" />
      <a-input
        data-testid="status" :value="controller.filters.value.status" allow-clear
        :placeholder="t('assets.resource.database.status')" :aria-label="t('assets.resource.database.status')" @change="setText('status', $event)" />
    </template>
    <a-input
      v-if="controller.family === 'firewall'" data-testid="network_ref" :value="controller.filters.value.network_ref" allow-clear
      :placeholder="t('assets.filters.network_ref')" :aria-label="t('assets.filters.network_ref')" @change="setText('network_ref', $event)"
    />
    <a-checkbox :checked="Boolean(controller.filters.value.include_deleted)" @change="setDeleted">
      {{ t('assets.resource.common.include_deleted') }}
    </a-checkbox>
    <span v-if="locationType" data-testid="location-context">{{ t(`assets.filters.location.${locationType}`) }}</span>
  </div>
</template>

<script setup lang="ts">
import type { AssetFilterState, AssetFiltersController } from '@/hooks/use-asset-filters'
import { useI18n } from '@/hooks/useI18n'
import type { LocationType, Provider } from '@/types/assets'

const props = defineProps<{ controller: AssetFiltersController; locationType?: LocationType }>()
const emit = defineEmits<{ change: [] }>()
const { t } = useI18n()
const providers = [{ value: 'aws', label: 'AWS' }, { value: 'aliyun', label: 'Aliyun' }, { value: 'gcp', label: 'GCP' }]
const commonFields = ['project_id', 'region', 'zone', 'q'] as const
const instanceFields = ['vpc_id', 'ip', 'tag_key', 'tag_value'] as const
const stateOptions = ['pending', 'running', 'stopping', 'stopped', 'suspended', 'terminated', 'error', 'unknown'].map(value => ({ value, label: value }))
function update(patch: Partial<AssetFilterState>) { props.controller.setFilters(patch); emit('change') }
function setProvider(value: Provider | undefined) { update({ provider: value }) }
function setAccount(value: number | string | null | undefined) { update({ account_id: value == null || value === '' ? undefined : Number(value) }) }
function setValue(field: 'state', value: string | undefined) { update({ [field]: value }) }
function setText(field: keyof AssetFilterState, event: Event) { update({ [field]: (event.target as HTMLInputElement).value.trim() || undefined }) }
function setDeleted(event: { target?: { checked?: boolean } }) { update({ include_deleted: event.target?.checked || undefined }) }
</script>

<style scoped>
.asset-filters { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
.asset-filters > :deep(.ant-select), .asset-filters > :deep(.ant-input), .asset-filters > :deep(.ant-input-number) { width: 180px; }
</style>
