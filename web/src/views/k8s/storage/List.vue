<template>
  <a-card class="resource-card" :title="$t('containers.storage')">
    <template #extra>
      <a-radio-group v-model:value="kind" class="resource-kind" :aria-label="$t('containers.resource_type')">
        <a-radio-button value="persistentvolumes">PV</a-radio-button>
        <a-radio-button value="persistentvolumeclaims">PVC</a-radio-button>
        <a-radio-button value="storageclasses">StorageClass</a-radio-button>
      </a-radio-group>
    </template>
    <a-space class="resource-toolbar" wrap>
      <ClusterPicker />
      <a-select v-if="kind === 'persistentvolumeclaims'" v-model:value="namespace" style="width: 220px" :placeholder="$t('k8s.cluster.namespace_all')" allow-clear>
        <a-select-option value="">{{ $t('k8s.cluster.namespace_all') }}</a-select-option>
        <a-select-option v-for="name in k8s.namespaces" :key="name" :value="name">{{ name }}</a-select-option>
      </a-select>
      <a-input v-model:value="search" allow-clear style="width: 240px" :placeholder="$t('containers.search')" />
      <a-button :loading="loading" @click="load"><ReloadOutlined />{{ $t('common.refresh') }}</a-button>
    </a-space>
    <a-alert v-if="k8s.currentClusterId === null" type="info" show-icon :message="$t('k8s.cluster.no_cluster_selected')" />
    <template v-else>
      <a-alert v-if="truncated" type="info" show-icon :message="$t('k8s.list.truncated', { limit: 500 })" />
      <a-table :key="kind" :columns="columns" :data-source="filtered" :loading="loading" :row-key="rowKey" :pagination="{ pageSize: 20, showSizeChanger: true }" :scroll="{ x: 'max-content' }" size="small" />
    </template>
  </a-card>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch } from 'vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import { message } from 'ant-design-vue'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import ClusterPicker from '@/components/layout/ClusterPicker.vue'
import { useK8sStore } from '@/stores/k8s'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from '@/hooks/useI18n'
import { usePageState } from '@/hooks/usePageState'
import { matchesResource, useResourceScope } from '@/hooks/useResourceScope'
import { isBizError } from '@/utils/http-error'
import type { K8sStorageApi, StorageKind, StorageSummary } from '@/api/k8s/storage'
import type { K8sNamespaceApi } from '@/api/k8s/namespace'

dayjs.extend(relativeTime)
const k8s = useK8sStore()
const auth = useAuthStore()
const { t } = useI18n()
const api = inject<K8sStorageApi>('k8sStorageApi')!
const namespaceApi = inject<K8sNamespaceApi>('k8sNsApi')!
const kind = ref<StorageKind>('persistentvolumes')
const search = ref('')
usePageState({ kind, search })
if (!['persistentvolumes', 'persistentvolumeclaims', 'storageclasses'].includes(kind.value)) kind.value = 'persistentvolumes'
const namespace = computed({ get: () => k8s.currentNamespace, set: value => k8s.setCurrentNamespace(value ?? '') })
const items = ref<StorageSummary[]>([])
const truncated = ref(false)
const loading = ref(false)
const captureScope = useResourceScope(kind)
const rowKey = (row: StorageSummary) => `${row.namespace ?? ''}/${row.name}`
const filtered = computed(() => items.value.filter(row => matchesResource(row, search.value)))
const columns = computed(() => {
  const fields = kind.value === 'persistentvolumes'
    ? ['name', 'capacity', 'access_modes', 'reclaim_policy', 'status', 'claim', 'storage_class']
    : kind.value === 'persistentvolumeclaims'
      ? ['name', 'namespace', 'capacity', 'access_modes', 'status', 'volume', 'storage_class']
      : ['name', 'provisioner', 'reclaim_policy', 'binding_mode']
  return [...fields.map(field => ({
    title: t(`containers.fields.${field}`), dataIndex: field,
    customRender: ({ text }: { text: unknown }) => Array.isArray(text) ? text.join(', ') || '-' : text || '-',
  })), { title: 'Age', dataIndex: 'age', customRender: ({ text }: { text: string }) => text ? dayjs(text).fromNow(true) : '-' }]
})
async function load() {
  const isCurrent = captureScope()
  const clusterId = k8s.currentClusterId
  items.value = []
  truncated.value = false
  loading.value = false
  if (clusterId === null || !auth.permissions.includes('k8s:storage:read')) return
  loading.value = true
  try {
    const result = await api.list(clusterId, kind.value, kind.value === 'persistentvolumeclaims' ? { namespace: k8s.currentNamespace || undefined } : {})
    if (!isCurrent()) return
    items.value = result.items
    truncated.value = result.truncated
  } catch (error) {
    if (isCurrent()) message.error(isBizError(error) ? error.message : t('network.error'))
  } finally {
    if (isCurrent()) loading.value = false
  }
}
watch([kind, () => k8s.currentClusterId, () => k8s.currentNamespace], () => {
  if (kind.value === 'persistentvolumeclaims' && auth.permissions.includes('k8s:cluster_resource:read')) {
    void k8s.ensureNamespaces(id => namespaceApi.list(id)).catch(() => undefined)
  }
  void load()
}, { immediate: true })
</script>
