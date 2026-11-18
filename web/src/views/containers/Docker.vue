<template>
  <div class="docker-page">
    <nav class="resource-navigation" :aria-label="$t('docker.resources')">
      <a-button
        v-for="section in available"
        :key="section.key"
        :type="active === section.key ? 'primary' : 'text'"
        :aria-current="active === section.key ? 'page' : undefined"
        @click="router.push(`/containers/docker/${section.key}`)"
      >
        <component :is="icons[section.key]" />
        {{ $t(section.label) }}
      </a-button>
    </nav>
    <div :key="auth.permissions.join('|')">
      <Hosts v-if="active === 'hosts'" />
      <Images v-else-if="active === 'images'" />
      <Containers v-else-if="active === 'containers'" />
      <a-empty v-else :description="$t('errors.403_title')" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CloudServerOutlined, BlockOutlined, AppstoreOutlined } from '@ant-design/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { dockerSections } from './sections'
import Hosts from './docker/Hosts.vue'
import Images from './docker/Images.vue'
import Containers from './docker/Containers.vue'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const available = computed(() => dockerSections.filter(section => section.permissions.some(permission => auth.permissions.includes(permission))))
const active = computed(() => {
  const requested = route.meta.dockerSection
  return requested ? available.value.find(section => section.key === requested)?.key : available.value[0]?.key
})
const icons = { hosts: CloudServerOutlined, images: BlockOutlined, containers: AppstoreOutlined }
</script>

<style scoped lang="scss">
.docker-page { display: flex; flex-direction: column; gap: 12px; }
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
