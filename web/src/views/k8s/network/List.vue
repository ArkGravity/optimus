<template>
  <div class="network-page">
    <a-card class="resource-card">
      <template #title>{{ $t('menu.k8s.network') }}</template>
      <template #extra>
        <a-radio-group v-model:value="currentKind" class="resource-kind" :aria-label="$t('containers.resource_type')">
          <a-radio-button v-for="kind in KINDS" :key="kind" :value="kind">{{ $t(`k8s.network.${KIND_SINGULAR[kind]}`) }}</a-radio-button>
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

          <template v-if="drawerKind === 'service'">
            <a-descriptions-item label="Type">
              {{ asService(detail)?.type ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item label="ClusterIP">
              {{ asService(detail)?.cluster_ip ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item
              label="Ports"
              :span="2"
            >
              <a-tag
                v-for="p in asService(detail)?.ports ?? []"
                :key="`${p.port}/${p.protocol}`"
                color="blue"
              >
                {{ p.port }}/{{ p.protocol }}
              </a-tag>
              <span v-if="!(asService(detail)?.ports?.length)">-</span>
            </a-descriptions-item>
            <a-descriptions-item
              v-if="(asService(detail)?.external_ips ?? []).length"
              label="ExternalIPs"
              :span="2"
            >
              {{ (asService(detail)?.external_ips ?? []).join(', ') }}
            </a-descriptions-item>
          </template>

          <template v-else-if="drawerKind === 'ingress'">
            <a-descriptions-item label="Class">
              {{ asIngress(detail)?.ingress_class ?? '-' }}
            </a-descriptions-item>
            <a-descriptions-item
              label="Hosts"
              :span="2"
            >
              <a-tag
                v-for="h in asIngress(detail)?.hosts ?? []"
                :key="h"
                color="geekblue"
              >
                {{ h }}
              </a-tag>
              <span v-if="!(asIngress(detail)?.hosts?.length)">-</span>
            </a-descriptions-item>
            <a-descriptions-item
              v-if="(asIngress(detail)?.load_balancer_ips ?? []).length"
              label="LoadBalancerIPs"
              :span="2"
            >
              {{ (asIngress(detail)?.load_balancer_ips ?? []).join(', ') }}
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
import type { K8sNetworkApi } from '@/api/k8s/network'
import type { K8sNamespaceApi } from '@/api/k8s/namespace'
import type {
  NetworkKind,
  NetworkSummary,
  ServiceSummary,
  IngressSummary,
} from '@/types/api'

dayjs.extend(relativeTime)

const k8s = useK8sStore()
const { t } = useI18n()
const { has } = usePermission()

const networkApi = inject<K8sNetworkApi>('k8sNetworkApi')!
const k8sNsApi = inject<K8sNamespaceApi>('k8sNsApi')!

const KINDS: NetworkKind[] = ['services', 'ingresses']

const KIND_SINGULAR: Record<NetworkKind, string> = {
  services: 'service',
  ingresses: 'ingress',
}

const currentKind = ref<NetworkKind>('services')
const items = ref<NetworkSummary[]>([])
const truncated = ref(false)
const loading = ref(false)
const search = ref('')
const filteredItems = computed(() => items.value.filter(row => matchesResource(row, search.value)))
const captureScope = useResourceScope(currentKind)

const drawerOpen = ref(false)
const drawerKind = ref<string>('')
const drawerDetail = ref<NetworkSummary | null>(null)

const namespaceModel = computed({
  get: () => k8s.currentNamespace,
  set: (v: string | undefined) => k8s.setCurrentNamespace(v ?? ''),
})

function rowKey(record: NetworkSummary) {
  return `${record.namespace}/${record.name}`
}

function detailRecord(detail: unknown): { name?: string; namespace?: string; age?: string } | undefined {
  return (detail ?? undefined) as { name?: string; namespace?: string; age?: string } | undefined
}

function asService(detail: unknown): ServiceSummary | undefined {
  return (detail ?? undefined) as ServiceSummary | undefined
}

function asIngress(detail: unknown): IngressSummary | undefined {
  return (detail ?? undefined) as IngressSummary | undefined
}

async function load(kind: NetworkKind): Promise<void> {
  if (!k8s.currentClusterId) return
  const isCurrent = captureScope()
  loading.value = true
  items.value = []
  truncated.value = false
  try {
    const res = await networkApi.list(k8s.currentClusterId, kind, {
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

function customRow(record: NetworkSummary) {
  return {
    onClick: () => void openDetail(record),
    style: { cursor: 'pointer' },
  }
}

async function openDetail(record: NetworkSummary): Promise<void> {
  if (!k8s.currentClusterId) return
  const isCurrent = captureScope('detail')
  const kind = KIND_SINGULAR[currentKind.value]
  try {
    const detail = await networkApi.get(
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
