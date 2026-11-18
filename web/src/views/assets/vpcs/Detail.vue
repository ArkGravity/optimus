<template>
  <AssetDetail family="vpc" :asset-key="assetKey" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AssetDetail from '../components/AssetDetail.vue'
const route = useRoute()
const assetKey = computed(() => typeof route.params.id === 'string' ? route.params.id : undefined)
</script>
