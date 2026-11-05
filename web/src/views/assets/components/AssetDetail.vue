<template>
  <a-card v-if="open" data-testid="asset-detail">
    <PageHeader :title="t(`assets.detail.sections.${family}`)"><slot name="actions" /></PageHeader>
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading">
      <template v-if="detail">
        <DetailSections :sections="sections" />
        <FirewallRules v-if="detail.family === 'firewall'" :firewall="detail.data" />
        <VPCSubnets v-if="detail.family === 'vpc'" :key="detail.data.asset_key" :asset-key="detail.data.asset_key" />
      </template>
    </a-spin>
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import type { AssetsResourceApi } from '@/api/assets/resource'
import PageHeader from '@/components/PageHeader.vue'
import { useI18n } from '@/hooks/useI18n'
import { usePermission } from '@/hooks/usePermission'
import { isBizError } from '@/utils/http-error'
import DetailSections from './DetailSections.vue'
import FirewallRules from './FirewallRules.vue'
import VPCSubnets from './VPCSubnets.vue'
import { detailSections, parseDetailKey, type DetailFamily, type DetailRecord } from './detail-fields'

const props = withDefaults(defineProps<{ family: DetailFamily; assetKey?: string | number; open?: boolean }>(), { open: true, assetKey: undefined })
const { t } = useI18n()
const { has } = usePermission()
const canRead = computed(() => has('assets:resource:read'))
const api = inject<AssetsResourceApi>('assetsResourceApi')!
const detail = shallowRef<DetailRecord>()
const loading = ref(false)
const error = ref('')
const sections = computed(() => detail.value ? detailSections(detail.value) : [])
let generation = 0
let alive = true

async function load() {
  const token = ++generation
  detail.value = undefined
  error.value = ''
  loading.value = false
  if (!props.open) return
  if (!canRead.value) { error.value = t('assets.detail.forbidden'); return }
  const key = parseDetailKey(props.assetKey, props.family)
  if (key === undefined) { error.value = t('assets.detail.invalid_key'); return }
  loading.value = true
  const current = () => alive && token === generation
  try {
    let result: DetailRecord
    const params = { include_deleted: true }
    switch (props.family) {
      case 'instance': result = { family: 'instance', data: await api.getInstance(key, params) }; break
      case 'vpc': result = { family: 'vpc', data: await api.getVPC(key, params) }; break
      case 'database': result = { family: 'database', data: await api.getDatabase(key, params) }; break
      case 'firewall': result = { family: 'firewall', data: await api.getFirewall(key, params) }; break
    }
    if (current()) detail.value = result
  } catch (cause) {
    if (current()) error.value = isBizError(cause) ? cause.message : t('network.error')
  } finally {
    if (current()) loading.value = false
  }
}
watch([() => props.family, () => props.assetKey, () => props.open, canRead], () => { void load() }, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { alive = false; generation += 1; detail.value = undefined; error.value = ''; loading.value = false })
</script>
