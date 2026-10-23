<template>
  <a-card>
    <PageHeader :title="$t('menu.delivery_group')" />
    <a-empty :description="$t('placeholder.reserved')" />
  </a-card>
</template>

<script setup lang="ts">
import PageHeader from '@/components/PageHeader.vue'
</script>
