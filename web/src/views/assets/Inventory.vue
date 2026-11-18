<template>
  <div class="inventory-page">
    <nav class="resource-navigation" :aria-label="$t('menu.assets.inventory')">
      <a-button
        v-for="section in available"
        :key="section.key"
        :type="active === section.key ? 'primary' : 'text'"
        :aria-current="active === section.key ? 'page' : undefined"
        @click="router.push(`/assets/inventory/${section.key}`)"
      >
        <component :is="icons[section.key]" />
        {{ $t(section.label) }}
      </a-button>
    </nav>
    <div :key="auth.permissions.join('|')">
      <Accounts v-if="active === 'accounts'" />
      <VPCs v-else-if="active === 'vpcs'" />
      <Firewalls v-else-if="active === 'firewalls'" />
      <Instances v-else-if="active === 'instances'" />
      <Databases v-else-if="active === 'databases'" />
      <SyncRuns v-else-if="active === 'sync-runs'" />
      <a-empty v-else :description="$t('errors.403_title')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CloudOutlined, ApartmentOutlined, SafetyOutlined, CloudServerOutlined, DatabaseOutlined, HistoryOutlined } from '@ant-design/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { inventorySections } from './sections'
import Accounts from '@/views/assets/cloud-accounts/List.vue'
import VPCs from '@/views/assets/vpcs/List.vue'
import Firewalls from '@/views/assets/firewalls/List.vue'
import Instances from '@/views/assets/instances/List.vue'
import Databases from '@/views/assets/databases/List.vue'
import SyncRuns from '@/views/assets/sync-runs/List.vue'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const available = computed(() => inventorySections.filter(section => section.permissions.some(permission => auth.permissions.includes(permission))))
const active = computed(() => {
  const requested = route.meta.assetsSection
  return requested ? available.value.find(section => section.key === requested)?.key : available.value[0]?.key
})
const icons = {
  accounts: CloudOutlined,
  vpcs: ApartmentOutlined,
  firewalls: SafetyOutlined,
  instances: CloudServerOutlined,
  databases: DatabaseOutlined,
  'sync-runs': HistoryOutlined,
}
</script>

<style scoped lang="scss">
.inventory-page { display: flex; flex-direction: column; gap: 12px; }
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
