<template>
  <div class="cluster-resources-page">
    <a-card class="resource-card">
      <template #title>{{ $t(section === 'nodes' ? 'containers.nodes' : 'containers.namespaces') }}</template>
      <template #extra>
        <a-radio-group v-if="section !== 'nodes'" v-model:value="currentKind" class="resource-kind" :aria-label="$t('containers.resource_type')">
          <a-radio-button value="namespaces">{{ $t('containers.namespaces') }}</a-radio-button>
          <a-radio-button value="events">{{ $t('k8s.cluster_resource.event') }}</a-radio-button>
        </a-radio-group>
      </template>
      <a-space class="resource-toolbar" wrap>
        <ClusterPicker />
        <!-- Namespace selector is only meaningful on the Events tab; for
             Namespace + Node it would be confusing (a namespace can't filter
             a list of namespaces). -->
        <a-select
          v-if="currentKind === 'events'"
          v-model:value="namespaceModel"
          :placeholder="$t('k8s.cluster.namespace_all')"
          style="width: 220px;"
          allow-clear
          @change="onNamespaceChange"
        >
          <a-select-option value="">{{ $t('k8s.cluster.namespace_all') }}</a-select-option>
          <a-select-option
            v-for="n in k8s.namespaces"
            :key="n"
            :value="n"
          >
            {{ n }}
          </a-select-option>
        </a-select>
        <a-input v-model:value="search" allow-clear :placeholder="$t('containers.search')" style="width: 240px" />
        <a-button
          :loading="loading"
          @click="load(currentKind)"
        >
          <template #icon>
            <reload-outlined />
          </template>
          {{ $t('common.refresh') }}
        </a-button>
      </a-space>

      <a-alert
        v-if="k8s.currentClusterId === null"
        :message="$t('k8s.cluster.no_cluster_selected')"
        type="info"
        show-icon
      />
      <template v-else>
        <a-alert v-if="truncated" :message="$t('k8s.list.truncated', { limit: 500 })" type="info" show-icon style="margin-bottom: 8px" />
          <a-table
            v-if="currentKind === 'namespaces'"
            :columns="namespaceColumns"
            :data-source="filteredItems"
            :loading="loading"
            :row-key="rowKeyName"
            :pagination="{ pageSize: 20, showSizeChanger: true }"
            :scroll="{ x: 'max-content' }"
            :custom-row="customRowNamespace"
            size="small"
          />
          <a-table
            v-if="currentKind === 'nodes'"
            :columns="nodeColumns"
            :data-source="filteredItems"
            :loading="loading"
            :row-key="rowKeyName"
            :pagination="{ pageSize: 20, showSizeChanger: true }"
            :scroll="{ x: 'max-content' }"
            :custom-row="customRowNode"
            size="small"
          />
          <a-table
            v-if="currentKind === 'events'"
            :columns="eventColumns"
            :data-source="filteredItems"
            :loading="loading"
            :row-key="rowKeyEvent"
            :pagination="{ pageSize: 20, showSizeChanger: true }"
            :scroll="{ x: 'max-content' }"
            size="small"
          />
      </template>
    </a-card>

    <ResourceDrawer
      v-if="drawerOpen && k8s.currentClusterId !== null"
      v-model:open="drawerOpen"
      :cluster-id="k8s.currentClusterId"
      :kind="drawerKind"
      :namespace="''"
      :name="drawerDetail?.name ?? ''"
      :detail="drawerDetail"
    >
      <template #overview="{ detail }">
        <a-descriptions
          :column="2"
          bordered
          size="small"
        >
          <a-descriptions-item :label="$t('k8s.cluster.name')">
            {{ detailRecord(detail)?.name ?? '-' }}
          </a-descriptions-item>

          <template v-if="drawerKind === 'namespace'">
            <a-descriptions-item label="Phase">
              {{ asNamespace(detail)?.phase ?? '-' }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'node'">
            <a-descriptions-item label="Ready">
              {{ asNode(detail)?.ready ? 'Yes' : 'No' }}
            </a-descriptions-item>
            <a-descriptions-item label="Schedulable">
              {{ asNode(detail)?.schedulable ? 'Yes' : 'No' }}
            </a-descriptions-item>
            <a-descriptions-item label="Roles">
              {{ (asNode(detail)?.roles ?? []).join(', ') || '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="KubeletVersion">
              {{ asNode(detail)?.kubelet_version ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="CPU">
              {{ asNode(detail)?.cpu_capacity ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="Mem">
              {{ asNode(detail)?.mem_capacity ?? '-' }}
            </a-descriptions-item>
          </template>

          <a-descriptions-item
            v-if="detailRecord(detail)?.age"
            label="Age"
          >
            {{ formatAge(detailRecord(detail)?.age) }}
          </a-descriptions-item>
        </a-descriptions>
      </template>
      <template #events>
        <a-empty :description="$t('placeholder.coming_soon')" />
      </template>
    </ResourceDrawer>
  </div>
</template>

<script setup lang="ts">
import { useResourceScope, matchesResource } from '@/hooks/useResourceScope'
import { usePageState } from '@/hooks/usePageState'
import { computed, inject, onBeforeMount, onBeforeUnmount, ref, watch } from 'vue'
import ClusterPicker from '@/components/layout/ClusterPicker.vue'
import { message } from 'ant-design-vue'
import { ReloadOutlined } from '@ant-design/icons-vue'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

import { usePermission } from '@/hooks/usePermission'
import { useI18n } from '@/hooks/useI18n'
import { useK8sStore } from '@/stores/k8s'
import { isBizError } from '@/utils/http-error'
import ResourceDrawer from '@/components/k8s/ResourceDrawer.vue'
import { eventColumns, namespaceColumns, nodeColumns } from './components/columns'
import type { K8sNamespaceApi } from '@/api/k8s/namespace'
import type { K8sNodeApi } from '@/api/k8s/node'
import type { K8sEventApi } from '@/api/k8s/event'
import type {
  EventSummary,
  NamespaceSummary,
  NodeSummary,
} from '@/types/api'

dayjs.extend(relativeTime)

type ClusterResourceKind = 'namespaces' | 'nodes' | 'events'

const k8s = useK8sStore()
const { t } = useI18n()
const { has } = usePermission()

const k8sNsApi = inject<K8sNamespaceApi>('k8sNsApi')!
const k8sNodeApi = inject<K8sNodeApi>('k8sNodeApi')!
const k8sEventApi = inject<K8sEventApi>('k8sEventApi')!

const { section = 'namespaces' } = defineProps<{ section?: 'nodes' | 'namespaces' }>()
const currentKind = ref<ClusterResourceKind>(section)
const items = ref<NamespaceSummary[] | NodeSummary[] | EventSummary[]>([])
const truncated = ref(false)
const loading = ref(false)
const search = ref('')
const filteredItems = computed(() => items.value.filter(row => matchesResource(row, search.value)))
const captureScope = useResourceScope(currentKind)

const drawerOpen = ref(false)
const drawerKind = ref<string>('')
const drawerDetail = ref<NamespaceSummary | NodeSummary | null>(null)

const namespaceModel = computed({
  get: () => k8s.currentNamespace,
  set: (v: string | undefined) => k8s.setCurrentNamespace(v ?? ''),
})

function rowKeyName(record: { name: string }) {
  return record.name
}

function rowKeyEvent(record: EventSummary, index?: number) {
  // Events don't have unique names — multiple Pods can emit the same
  // (reason, involved_kind, involved_name) tuple. Index keeps each row
  // distinct without warnings.
  return `${record.namespace ?? ''}/${record.involved_kind}/${record.involved_name}/${record.reason}/${index ?? 0}`
}

function detailRecord(detail: unknown): { name?: string; age?: string } | undefined {
  return (detail ?? undefined) as { name?: string; age?: string } | undefined
}

function asNamespace(detail: unknown): NamespaceSummary | undefined {
  return (detail ?? undefined) as NamespaceSummary | undefined
}

function asNode(detail: unknown): NodeSummary | undefined {
  return (detail ?? undefined) as NodeSummary | undefined
}

async function load(kind: ClusterResourceKind): Promise<void> {
  if (!k8s.currentClusterId) return
  const isCurrent = captureScope()
  loading.value = true
  items.value = []
  truncated.value = false
  try {
    if (kind === 'namespaces') {
      const res = await k8sNsApi.list(k8s.currentClusterId)
      if (!isCurrent()) return
      items.value = res.items
      truncated.value = res.truncated
    } else if (kind === 'nodes') {
      const res = await k8sNodeApi.list(k8s.currentClusterId)
      if (!isCurrent()) return
      items.value = res.items
      truncated.value = res.truncated
    } else {
      const res = await k8sEventApi.list(k8s.currentClusterId, {
        namespace: k8s.currentNamespace || undefined,
      })
      if (!isCurrent()) return
      items.value = res.items
      truncated.value = res.truncated
    }
  } catch (e) {
    if (isCurrent()) message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    if (isCurrent()) loading.value = false
  }
}

function customRowNamespace(record: NamespaceSummary) {
  return {
    onClick: () => void openNamespace(record),
    style: { cursor: 'pointer' },
  }
}

function customRowNode(record: NodeSummary) {
  return {
    onClick: () => void openNode(record),
    style: { cursor: 'pointer' },
  }
}

function openNamespace(record: NamespaceSummary): void {
  drawerKind.value = 'namespace'
  drawerDetail.value = record
  drawerOpen.value = true
}

async function openNode(record: NodeSummary): Promise<void> {
  if (!k8s.currentClusterId) return
  const isCurrent = captureScope('detail')
  const kind = 'node'
  try {
    const detail = await k8sNodeApi.get(k8s.currentClusterId, record.name)
    if (!isCurrent()) return
    drawerKind.value = kind
    drawerDetail.value = detail
    drawerOpen.value = true
  } catch (e) {
    if (isCurrent()) message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

function onNamespaceChange(): void {
  void load(currentKind.value)
}

function formatAge(iso?: string): string {
  if (!iso) return '-'
  return dayjs(iso).fromNow(true)
}

usePageState({ currentKind, search })
if (section === 'nodes') currentKind.value = 'nodes'
else if (currentKind.value === 'nodes') currentKind.value = 'namespaces'

watch([currentKind, () => k8s.currentNamespace, () => k8s.currentClusterId], () => {
  drawerOpen.value = false
  drawerDetail.value = null
  items.value = []
  loading.value = false
  truncated.value = false
}, { flush: 'sync' })
watch(currentKind, kind => void load(kind))
watch(
  () => k8s.currentClusterId,
  (id) => {
    drawerOpen.value = false
    drawerDetail.value = null
    items.value = []
    if (id !== null) {
      if (has('k8s:cluster_resource:read')) void k8s.ensureNamespaces(cid => k8sNsApi.list(cid)).catch(() => undefined)
      void load(currentKind.value)
    }
  },
)

onBeforeUnmount(() => {
  drawerOpen.value = false
  drawerDetail.value = null
})

onBeforeMount(() => {
  if (k8s.currentClusterId === null) {
    return
  }
  // Namespace cache is needed for the Events tab filter.
  if (has('k8s:cluster_resource:read')) void k8s.ensureNamespaces(cid => k8sNsApi.list(cid)).catch(() => undefined)
  void load(currentKind.value)
})
</script>
