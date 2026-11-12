<template>
  <div class="kubernetes-page">
    <nav class="resource-navigation" :aria-label="$t('containers.resources')">
      <a-button
        v-for="section in available"
        :key="section.key"
        :type="active === section.key ? 'primary' : 'text'"
        :aria-current="active === section.key ? 'page' : undefined"
        @click="router.push(`/containers/kubernetes/${section.key}`)"
      >
        <component :is="icons[section.key]" />
        {{ $t(section.label) }}
      </a-button>
    </nav>
    <div :key="auth.permissions.join('|')">
      <Clusters v-if="active === 'clusters'" />
      <ClusterResources v-else-if="active === 'nodes' || active === 'namespaces'" :key="active" :section="active" />
      <Workloads v-else-if="active === 'workloads'" />
      <Network v-else-if="active === 'network'" />
      <Storage v-else-if="active === 'storage'" />
      <Configs v-else-if="active === 'configs'" />
      <a-empty v-else :description="$t('errors.403_title')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ClusterOutlined, HddOutlined, FolderOutlined, AppstoreOutlined, ShareAltOutlined, DatabaseOutlined, SettingOutlined } from '@ant-design/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { kubernetesSections } from './sections'
import Clusters from '@/views/k8s/clusters/List.vue'
import ClusterResources from '@/views/k8s/cluster-resources/List.vue'
import Workloads from '@/views/k8s/workloads/List.vue'
import Network from '@/views/k8s/network/List.vue'
import Configs from '@/views/k8s/config/List.vue'
import Storage from '@/views/k8s/storage/List.vue'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const available = computed(() => kubernetesSections.filter(section => section.permissions.some(permission => auth.permissions.includes(permission))))
const active = computed(() => {
  const requested = route.meta.kubernetesSection
  return requested ? available.value.find(section => section.key === requested)?.key : available.value[0]?.key
})
const icons = { clusters: ClusterOutlined, nodes: HddOutlined, namespaces: FolderOutlined, workloads: AppstoreOutlined, network: ShareAltOutlined, storage: DatabaseOutlined, configs: SettingOutlined }
</script>

<style scoped lang="scss">
.kubernetes-page { display: flex; flex-direction: column; gap: 12px; }
.resource-navigation {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid var(--ant-color-border-secondary);
  border-radius: var(--ant-border-radius-lg, 8px);
  background: var(--ant-color-bg-container);
  .ant-btn { height: 38px; }
}
</style>
