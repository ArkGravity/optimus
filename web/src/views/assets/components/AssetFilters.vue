<template>
  <div class="asset-filters">
    <div class="filter-row">
      <span class="filter-control">
        <a-select
          data-testid="provider" :value="controller.filters.value.provider" :options="providers"
          allow-clear :placeholder="t('assets.filters.provider')" :aria-label="t('assets.filters.provider')"
          @change="setProvider"
        />
      </span>
      <span class="filter-control">
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
      </span>
      <span class="filter-control filter-control--search">
        <a-input
          data-testid="q" :value="controller.filters.value.q"
          allow-clear :placeholder="t('assets.filters.q')" :aria-label="t('assets.filters.q')"
          @change="setText('q', $event)"
        />
      </span>
      <a-checkbox :checked="Boolean(controller.filters.value.include_deleted)" @change="setDeleted">
        {{ t('assets.resource.common.include_deleted') }}
      </a-checkbox>
      <button
        type="button"
        class="more-filters"
        :class="{ 'more-filters--open': moreOpen }"
        data-testid="more-filters"
        :aria-expanded="moreOpen"
        @click="moreOpen = !moreOpen"
      >
        <DownOutlined class="more-filters-icon" :class="{ 'more-filters-icon--open': moreOpen }" />
        <span>{{ t('assets.filters.more') }}</span>
        <span v-if="advancedCount && !moreOpen" class="more-filters-count">{{ advancedCount }}</span>
      </button>
      <span v-if="locationType" data-testid="location-context">{{ t(`assets.filters.location.${locationType}`) }}</span>
    </div>
    <div v-show="moreOpen" class="filter-row more-filter-fields" data-testid="more-filter-fields">
      <span v-for="field in scopeFields" :key="field" class="filter-control">
        <a-input
          :data-testid="field" :value="controller.filters.value[field]"
          allow-clear :placeholder="t(`assets.filters.${field}`)" :aria-label="t(`assets.filters.${field}`)"
          @change="setText(field, $event)"
        />
      </span>
      <template v-if="controller.family === 'instance'">
        <span class="filter-control">
          <a-select
            data-testid="state" :value="controller.filters.value.state" :options="stateOptions" allow-clear
            :placeholder="t('assets.resource.instance.state')" :aria-label="t('assets.resource.instance.state')"
            @change="setValue('state', $event)"
          />
        </span>
        <span v-for="field in instanceFields" :key="field" class="filter-control">
          <a-input
            :data-testid="field" :value="controller.filters.value[field]"
            allow-clear :placeholder="t(`assets.filters.${field}`)" :aria-label="t(`assets.filters.${field}`)"
            @change="setText(field, $event)"
          />
        </span>
      </template>
      <template v-if="controller.family === 'database'">
        <span class="filter-control">
          <a-input
            data-testid="engine" :value="controller.filters.value.engine" allow-clear
            :placeholder="t('assets.resource.database.engine')" :aria-label="t('assets.resource.database.engine')" @change="setText('engine', $event)"
          />
        </span>
        <span class="filter-control">
          <a-input
            data-testid="status" :value="controller.filters.value.status" allow-clear
            :placeholder="t('assets.resource.database.status')" :aria-label="t('assets.resource.database.status')" @change="setText('status', $event)"
          />
        </span>
      </template>
      <span v-if="controller.family === 'firewall'" class="filter-control">
        <a-input
          data-testid="network_ref" :value="controller.filters.value.network_ref" allow-clear
          :placeholder="t('assets.filters.network_ref')" :aria-label="t('assets.filters.network_ref')" @change="setText('network_ref', $event)"
        />
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { DownOutlined } from '@ant-design/icons-vue'
import type { AssetFilterState, AssetFiltersController } from '@/hooks/use-asset-filters'
import { useI18n } from '@/hooks/useI18n'
import type { LocationType, Provider } from '@/types/assets'

const props = defineProps<{ controller: AssetFiltersController; locationType?: LocationType }>()
const emit = defineEmits<{ change: [] }>()
const { t } = useI18n()
const providers = [{ value: 'aws', label: 'AWS' }, { value: 'aliyun', label: 'Aliyun' }, { value: 'gcp', label: 'GCP' }]
const scopeFields = ['project_id', 'region', 'zone'] as const
const instanceFields = ['vpc_id', 'ip', 'tag_key', 'tag_value'] as const
const stateOptions = ['pending', 'running', 'stopping', 'stopped', 'suspended', 'terminated', 'error', 'unknown'].map(value => ({ value, label: value }))
const advancedFields = computed(() => {
  const fields: Array<keyof AssetFilterState> = [...scopeFields]
  if (props.controller.family === 'instance') fields.push('state', ...instanceFields)
  if (props.controller.family === 'database') fields.push('engine', 'status')
  if (props.controller.family === 'firewall') fields.push('network_ref')
  return fields
})
const advancedCount = computed(() => advancedFields.value.filter(field => {
  const value = props.controller.filters.value[field]
  return value !== undefined && value !== ''
}).length)
const moreOpen = ref(advancedCount.value > 0)
function update(patch: Partial<AssetFilterState>) { props.controller.setFilters(patch); emit('change') }
function setProvider(value: Provider | undefined) { update({ provider: value }) }
function setAccount(value: number | string | null | undefined) { update({ account_id: value == null || value === '' ? undefined : Number(value) }) }
function setValue(field: 'state', value: string | undefined) { update({ [field]: value }) }
function setText(field: keyof AssetFilterState, event: Event) { update({ [field]: (event.target as HTMLInputElement).value.trim() || undefined }) }
function setDeleted(event: { target?: { checked?: boolean } }) { update({ include_deleted: event.target?.checked || undefined }) }
</script>

<style scoped>
.asset-filters { display: flex; flex-direction: column; gap: 12px; }
.filter-row { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
.filter-control { flex: 0 0 168px; width: 168px; }
.filter-control--search { flex-basis: 240px; width: 240px; }
.filter-control :deep(.ant-select),
.filter-control :deep(.ant-input),
.filter-control :deep(.ant-input-affix-wrapper),
.filter-control :deep(.ant-input-number) { width: 100%; }
.more-filters {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 10px;
  border: 1px solid var(--ant-color-border);
  border-radius: 6px;
  background: var(--ant-color-fill-quaternary);
  color: var(--ant-color-text);
  cursor: pointer;
}
.more-filters--open {
  border-color: var(--ant-color-primary);
  background: var(--ant-color-primary-bg);
  color: var(--ant-color-primary);
}
.more-filters-icon { font-size: 12px; transition: transform 0.15s ease; }
.more-filters-icon--open { transform: rotate(180deg); }
.more-filters-count {
  min-width: 16px;
  padding: 0 5px;
  border-radius: 8px;
  background: var(--ant-color-primary);
  color: var(--ant-color-text-light-solid, #fff);
  font-size: 12px;
  line-height: 16px;
  text-align: center;
}
.more-filter-fields {
  padding: 12px;
  border: 1px solid var(--ant-color-border-secondary);
  border-radius: 8px;
  background: var(--ant-color-fill-quaternary);
}
</style>
