<template>
  <section v-for="section in sections" :key="section.key" class="detail-section" :data-section="section.key">
    <h3>{{ t(`assets.detail.sections.${section.title}`) }}</h3>
    <p v-if="section.key === 'database-native' || section.title.startsWith('database_provenance_')" class="detail-note">{{ t('assets.detail.provenance_notice') }}</p>
    <dl class="detail-fields">
      <div v-for="field in section.fields" :key="field.label" :data-field="field.label">
        <dt>{{ t(`assets.detail.fields.${field.label}`) }}</dt>
        <dd>
          <ul v-if="Array.isArray(field.value) && field.value.length"><li v-for="(value, index) in field.value" :key="index">{{ value }}</li></ul>
          <template v-else-if="field.label === 'location_type' && typeof field.value === 'string'">{{ t(`assets.filters.location.${field.value}`) }}</template>
          <template v-else-if="typeof field.value === 'boolean'">{{ t(`assets.detail.${field.value ? 'yes' : 'no'}`) }}</template>
          <template v-else>{{ display(field.value) }}</template>
        </dd>
      </div>
    </dl>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from '@/hooks/useI18n'
import type { DetailField, DetailSection } from './detail-fields'
defineProps<{ sections: DetailSection[] }>()
const { t } = useI18n()
function display(value: DetailField['value']) { return value == null || value === '' || Array.isArray(value) ? t('assets.detail.not_applicable') : value }
</script>

<style scoped>
.detail-section { margin-block: 16px; }
h3 { font-size: 14px; margin-bottom: 12px; }
.detail-fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(280px, 100%), 1fr)); gap: 12px 20px; margin: 0; }
dt, .detail-note { color: var(--ant-color-text-secondary); }
dt { font-size: 12px; margin-bottom: 4px; }
dd { margin: 0; overflow-wrap: anywhere; }
ul { list-style: none; margin: 0; padding: 0; }
</style>
