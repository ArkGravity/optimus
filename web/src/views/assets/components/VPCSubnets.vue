<template>
  <section>
    <h3>{{ t('assets.resource.subnet.title') }}</h3>
    <div class="subnet-filters u-mb-16">
      <a-input-search v-model:value="searchDraft" allow-clear :placeholder="t('assets.resource.subnet.subnet_id')" @search="search" @change="onSearchChange" />
      <a-checkbox :checked="includeDeleted" @change="setDeleted">{{ t('assets.resource.common.include_deleted') }}</a-checkbox>
    </div>
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading">
      <p v-if="!loading && !error && !items.length">{{ t('assets.resource.common.no_data') }}</p>
      <DetailSections :sections="sections" />
    </a-spin>
    <a-pagination :current="page" :page-size="size" :total="total" show-size-changer @change="setPagination" />
  </section>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import type { AssetsResourceApi } from '@/api/assets/resource'
import type { AssetKey, SubnetSummary } from '@/types/assets'
import { useI18n } from '@/hooks/useI18n'
import { isBizError } from '@/utils/http-error'
import DetailSections from './DetailSections.vue'
import { subnetSection } from './detail-fields'
const props = defineProps<{ assetKey: AssetKey }>()
const api = inject<AssetsResourceApi>('assetsResourceApi')!
const { t } = useI18n()
const searchDraft = ref('')
const query = ref('')
const includeDeleted = ref(false)
const page = ref(1)
const size = ref(20)
const total = ref(0)
const items = shallowRef<SubnetSummary[]>([])
const loading = ref(false)
const error = ref('')
const sections = computed(() => items.value.map(subnetSection))
let generation = 0
let alive = true
async function load() {
  const token = ++generation
  items.value = []; total.value = 0; error.value = ''; loading.value = true
  const params = { page: page.value, size: size.value, ...(query.value ? { q: query.value } : {}), ...(includeDeleted.value ? { include_deleted: true } : {}) }
  try {
    const result = await api.listSubnets(props.assetKey, params)
    if (alive && token === generation) { items.value = result.items; total.value = result.total }
  } catch (cause) {
    if (alive && token === generation) error.value = isBizError(cause) ? cause.message : t('network.error')
  } finally { if (alive && token === generation) loading.value = false }
}
function search(value: string) {
  searchDraft.value = value
  const next = value.trim()
  if (!next && !query.value && page.value === 1) return
  query.value = next; page.value = 1; void load()
}
function onSearchChange(event: Event) { if ((event.target as HTMLInputElement).value === '') search('') }
function setDeleted(event: { target?: { checked?: boolean } }) { includeDeleted.value = Boolean(event.target?.checked); page.value = 1; void load() }
function setPagination(value: number, pageSize = size.value) {
  page.value = pageSize === size.value ? value : 1
  size.value = pageSize
  void load()
}
watch(() => props.assetKey, () => { searchDraft.value = ''; query.value = ''; includeDeleted.value = false; page.value = 1; size.value = 20; void load() }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { alive = false; generation += 1; searchDraft.value = ''; query.value = ''; items.value = []; total.value = 0; error.value = ''; loading.value = false })
</script>

<style scoped>
h3 { font-size: 14px; }
.subnet-filters { display: flex; flex-wrap: wrap; gap: 12px; align-items: center; }
.subnet-filters > :first-child { width: 260px; }
</style>
