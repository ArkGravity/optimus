<template>
  <AssetDetail family="vpc" :asset-key="assetKey">
    <template #actions><a-button @click="router.back()">{{ t('common.button.back') }}</a-button></template>
  </AssetDetail>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AssetDetail from '../components/AssetDetail.vue'
import { useI18n } from '@/hooks/useI18n'
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const assetKey = computed(() => typeof route.params.id === 'string' ? route.params.id : undefined)
</script>
