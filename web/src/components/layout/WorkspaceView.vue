<template>
  <router-view v-slot="{ Component, route: current }">
    <PageSession :key="current.path" :page-key="current.meta.menuName ? current.path : undefined">
      <component :is="Component" />
    </PageSession>
  </router-view>
</template>

<script setup lang="ts">
import PageSession from './PageSession.vue'
</script>
