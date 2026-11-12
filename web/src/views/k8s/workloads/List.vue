<template>
  <div class="workloads-page">
    <a-card class="resource-card">
      <template #title>{{ $t('menu.k8s.workloads') }}</template>
      <template #extra>
        <a-radio-group v-model:value="currentKind" class="resource-kind" :aria-label="$t('containers.resource_type')">
          <a-radio-button v-for="kind in KINDS" :key="kind" :value="kind">{{ $t(`k8s.workload.${KIND_SINGULAR[kind]}`) }}</a-radio-button>
        </a-radio-group>
      </template>
      <a-space class="resource-toolbar" wrap>
        <ClusterPicker />
        <a-select
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
            :key="currentKind"
            :columns="columnsByKind[currentKind]"
            :data-source="filteredItems"
            :loading="loading"
            :row-key="rowKey"
            :pagination="{ pageSize: 20, showSizeChanger: true }"
            :scroll="{ x: 'max-content' }"
            :custom-row="customRow"
            size="small"
          />
      </template>
    </a-card>

    <ResourceDrawer
      v-if="drawerOpen && k8s.currentClusterId !== null"
      v-model:open="drawerOpen"
      :cluster-id="k8s.currentClusterId"
      :kind="drawerKind"
      :namespace="drawerDetail?.namespace ?? ''"
      :name="drawerDetail?.name ?? ''"
      :detail="drawerDetail"
      :pod-containers="podContainers"
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
          <a-descriptions-item :label="$t('k8s.cluster.namespace')">
            {{ detailRecord(detail)?.namespace ?? '-' }}
          </a-descriptions-item>

          <template v-if="drawerKind === 'pod'">
            <a-descriptions-item label="Phase">
              {{ detailRecord(detail)?.phase ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="Ready">
              {{ detailRecord(detail)?.ready_containers ?? 0 }}/{{ detailRecord(detail)?.total_containers ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Restarts">
              {{ detailRecord(detail)?.restart_count ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Node">
              {{ detailRecord(detail)?.node_name ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="IP">
              {{ detailRecord(detail)?.pod_ip ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item
              v-if="detailRecord(detail)?.status_reason"
              label="Reason"
            >
              {{ detailRecord(detail)?.status_reason }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'deployment'">
            <a-descriptions-item label="Replicas">
              {{ detailRecord(detail)?.replicas_ready ?? 0 }}/{{ detailRecord(detail)?.replicas_desired ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Updated">
              {{ detailRecord(detail)?.replicas_updated ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Available">
              {{ detailRecord(detail)?.replicas_available ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Strategy">
              {{ detailRecord(detail)?.strategy ?? '-' }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'statefulset'">
            <a-descriptions-item label="Replicas">
              {{ detailRecord(detail)?.ready_replicas ?? 0 }}/{{ detailRecord(detail)?.replicas ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Service">
              {{ detailRecord(detail)?.service_name ?? '-' }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'daemonset'">
            <a-descriptions-item label="Desired">
              {{ detailRecord(detail)?.desired_number ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Current">
              {{ detailRecord(detail)?.current_number ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Ready">
              {{ detailRecord(detail)?.ready_number ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Available">
              {{ detailRecord(detail)?.available_number ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Misscheduled">
              {{ detailRecord(detail)?.misscheduled ?? 0 }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'job'">
            <a-descriptions-item label="Completions">
              {{ detailRecord(detail)?.completions ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Succeeded">
              {{ detailRecord(detail)?.succeeded ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Failed">
              {{ detailRecord(detail)?.failed ?? 0 }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'cronjob'">
            <a-descriptions-item label="Schedule">
              {{ detailRecord(detail)?.schedule ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="Active">
              {{ detailRecord(detail)?.active_jobs ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item label="Suspended">
              {{ detailRecord(detail)?.suspended ? 'Yes' : 'No' }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'replicaset'">
            <a-descriptions-item label="Replicas">
              {{ detailRecord(detail)?.ready_replicas ?? 0 }}/{{ detailRecord(detail)?.replicas ?? 0 }}
            </a-descriptions-item>
            <a-descriptions-item
              v-if="detailRecord(detail)?.owner_kind"
              label="Owner"
            >
              {{ detailRecord(detail)?.owner_kind }}/{{ detailRecord(detail)?.owner_name }}
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
import { columnsByKind } from './components/columns'
import type { WorkloadApi } from '@/api/k8s/workload'
import type { K8sNamespaceApi } from '@/api/k8s/namespace'
import type { PodSummary, WorkloadKind, WorkloadSummary } from '@/types/api'

dayjs.extend(relativeTime)

const k8s = useK8sStore()
const { t } = useI18n()
const { has } = usePermission()

const workloadApi = inject<WorkloadApi>('workloadApi')!
const k8sNsApi = inject<K8sNamespaceApi>('k8sNsApi')!

const KINDS: WorkloadKind[] = [
  'deployments',
  'statefulsets',
  'daemonsets',
  'jobs',
  'cronjobs',
  'replicasets',
  'pods',
]

// Plural URL kind -> singular drawer kind, used by both the i18n tab label
// lookup and the ResourceDrawer's `kind` prop (singular is what the YAML +
// log endpoints expect).
const KIND_SINGULAR: Record<WorkloadKind, string> = {
  deployments: 'deployment',
  statefulsets: 'statefulset',
  daemonsets: 'daemonset',
  jobs: 'job',
  cronjobs: 'cronjob',
  replicasets: 'replicaset',
  pods: 'pod',
}

const currentKind = ref<WorkloadKind>('deployments')
const items = ref<WorkloadSummary[]>([])
const truncated = ref(false)
const loading = ref(false)
const search = ref('')
const filteredItems = computed(() => items.value.filter(row => matchesResource(row, search.value)))
const captureScope = useResourceScope(currentKind)

const drawerOpen = ref(false)
const drawerKind = ref<string>('')
const drawerDetail = ref<WorkloadSummary | null>(null)

// Two-way binding helper — `useK8sStore().currentNamespace` is the source of
// truth, but the select needs a plain ref to mutate. We forward via setter.
const namespaceModel = computed({
  get: () => k8s.currentNamespace,
  set: (v: string | undefined) => k8s.setCurrentNamespace(v ?? ''),
})

function rowKey(record: WorkloadSummary) {
  // Names are unique within a (kind, namespace) tuple; namespace+name avoids
  // duplicate-key warnings when a cluster-wide list returns same-named items
  // from different namespaces.
  return `${record.namespace}/${record.name}`
}

function detailRecord(detail: unknown): Record<string, never> | undefined {
  // The drawer slot exposes `detail` as `unknown`; this narrows it for the
  // template without leaking `any`. We accept that fields not present on a
  // given kind render as undefined (then masked to '-' by the template).
  return (detail ?? undefined) as Record<string, never> | undefined
}

const podContainers = computed<string[]>(() => {
  if (drawerKind.value !== 'pod') return []
  const d = drawerDetail.value as PodSummary | null
  if (!d) return []
  // PodSummary doesn't expose a container list; the page only knows how many
  // containers are ready. For v1 we hand LogViewer an empty list — it falls
  // back to the default container.
  return []
})

async function load(kind: WorkloadKind): Promise<void> {
  if (!k8s.currentClusterId) return
  const isCurrent = captureScope()
  loading.value = true
  items.value = []
  truncated.value = false
  try {
    const res = await workloadApi.list(k8s.currentClusterId, kind, {
      namespace: k8s.currentNamespace || undefined,
    })
    if (!isCurrent()) return
    items.value = res.items
    truncated.value = res.truncated
  } catch (e) {
    if (isCurrent()) message.error(isBizError(e) ? e.message : t('network.error'))
  } finally {
    if (isCurrent()) loading.value = false
  }
}

function customRow(record: WorkloadSummary) {
  return {
    onClick: () => void openDetail(record),
    style: { cursor: 'pointer' },
  }
}

async function openDetail(record: WorkloadSummary): Promise<void> {
  if (!k8s.currentClusterId) return
  const isCurrent = captureScope('detail')
  const kind = KIND_SINGULAR[currentKind.value]
  try {
    const detail = await workloadApi.get(
      k8s.currentClusterId,
      currentKind.value,
      record.namespace,
      record.name,
    )
    if (!isCurrent()) return
    drawerKind.value = kind
    drawerDetail.value = detail
    drawerOpen.value = true
  } catch (e) {
    if (isCurrent()) message.error(isBizError(e) ? e.message : t('network.error'))
  }
}

function onNamespaceChange(): void {
  // `currentNamespace` setter is invoked via v-model; an explicit reload here
  // makes the dependency from selector -> table data unambiguous.
  void load(currentKind.value)
}

function formatAge(iso?: string): string {
  if (!iso) return '-'
  return dayjs(iso).fromNow(true)
}

usePageState({ currentKind, search })

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
  if (has('k8s:cluster_resource:read')) void k8s.ensureNamespaces(cid => k8sNsApi.list(cid)).catch(() => undefined)
  void load(currentKind.value)
})
</script>
