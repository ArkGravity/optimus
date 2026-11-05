<template>
  <section>
    <p class="inventory-note">{{ t('assets.detail.firewall_inventory_notice') }}</p>
    <section v-for="direction in directions" :key="direction">
      <h3>{{ t(`assets.resource.security_group.${direction}`) }}</h3>
      <p v-if="!firewall[`${direction}_rules`].length">{{ t('assets.resource.common.no_data') }}</p>
      <article v-for="(rule, index) in firewall[`${direction}_rules`]" :key="index" class="firewall-rule" :data-rule="`${direction}-${index}`">
        <dl class="rule-fields">
          <div><dt>{{ t('assets.resource.security_group.protocol') }}</dt><dd>{{ formatRuleProtocol(rule, labels.all) }}</dd></div>
          <div><dt>{{ t('assets.resource.security_group.ports') }}</dt><dd data-rule-field="ports">{{ formatRulePorts(rule, labels, facts(direction, index).aliyun) }}</dd></div>
          <div><dt>{{ t('assets.firewall_rules.action') }}</dt><dd>{{ formatRuleAction(rule, labels) }}</dd></div>
          <div><dt>{{ t('assets.firewall_rules.priority') }}</dt><dd data-rule-field="priority">{{ formatRulePriority(rule, labels.unavailable) }}</dd></div>
          <div><dt>{{ t('assets.detail.fields.enabled') }}</dt><dd>{{ formatRuleEnabled(rule, labels) }}</dd></div>
          <div><dt>{{ t('assets.detail.fields.description') }}</dt><dd>{{ rule.description || labels.unavailable }}</dd></div>
          <div v-for="target in ruleTargets(rule)" :key="target.kind"><dt>{{ targetLabel(target.kind) }}</dt><dd><ul><li v-for="(value, targetIndex) in target.values" :key="targetIndex">{{ value }}</li></ul></dd></div>
        </dl>
        <DetailSections :sections="nativeSections(direction, index)" />
      </article>
    </section>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from '@/hooks/useI18n'
import type { AliyunRuleFacts, AWSRuleFacts, FirewallDetail } from '@/types/assets'
import { formatRuleAction, formatRuleEnabled, formatRulePorts, formatRulePriority, formatRuleProtocol, ruleNativeFacts, ruleTargets } from '../firewalls/rules'
import DetailSections from './DetailSections.vue'
import type { DetailSection } from './detail-fields'
const props = defineProps<{ firewall: FirewallDetail }>()
const { t } = useI18n()
const directions = ['inbound', 'outbound'] as const
const labels = computed(() => ({ all: t('assets.firewall_rules.all'), unavailable: t('assets.detail.not_applicable'), allow: t('assets.firewall_rules.allow'), deny: t('assets.firewall_rules.deny'), enabled: t('assets.firewall_rules.enabled'), disabled: t('assets.firewall_rules.disabled'), portList: t('assets.firewall_rules.port_list') }))
const awsFields = ['owner_id', 'vpc_id', 'peering_id'] as const satisfies readonly (keyof AWSRuleFacts)[]
const aliyunFields = ['source_group_id', 'destination_group_id', 'source_prefix_list_id', 'destination_prefix_list_id', 'rule_id', 'nic_type', 'source_group_name', 'source_group_owner_account', 'destination_group_name', 'destination_group_owner_account', 'source_prefix_list_name', 'destination_prefix_list_name', 'source_port_range', 'port_range_list_id', 'port_range_list_name'] as const satisfies readonly (keyof AliyunRuleFacts)[]
function facts(direction: 'inbound' | 'outbound', index: number) { return ruleNativeFacts(props.firewall.native_details, direction, index) }
function nativeSections(direction: 'inbound' | 'outbound', index: number): DetailSection[] {
  const native = facts(direction, index)
  const fields = [...(native.aws ? awsFields.map(label => ({ label, value: native.aws![label] })) : []), ...(native.aliyun ? aliyunFields.map(label => ({ label, value: native.aliyun![label] })) : [])]
  return fields.length ? [{ key: `${direction}-${index}-native`, title: 'rule_native', fields }] : []
}
function targetLabel(kind: string) {
  const known = ['ipv4', 'ipv6', 'security_group', 'prefix_list', 'source_ranges', 'destination_ranges', 'source_network_tags', 'target_network_tags', 'source_service_accounts', 'target_service_accounts']
  return known.includes(kind) ? t(`assets.firewall_rules.${kind}`) : kind
}
</script>

<style scoped>
.inventory-note, dt { color: var(--ant-color-text-secondary); }
h3 { font-size: 14px; }
.firewall-rule { border: 1px solid var(--ant-color-border); border-radius: 6px; padding: 12px; margin-bottom: 12px; }
.rule-fields { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(220px, 100%), 1fr)); gap: 12px; }
dt { font-size: 12px; }
dd { margin: 4px 0 0; overflow-wrap: anywhere; }
ul { list-style: none; margin: 0; padding: 0; }
</style>
