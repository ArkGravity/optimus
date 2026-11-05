import type { AliyunRuleFacts, AWSRuleFacts, FirewallNativeDetails, FirewallRule } from '@/types/assets'

export interface RuleLabels {
  all: string
  unavailable: string
  allow: string
  deny: string
  enabled: string
  disabled: string
  portList: string
}
export function formatRuleProtocol(rule: FirewallRule, all: string): string {
  const names: Record<string, string> = { '-1': all, all, '6': 'TCP', tcp: 'TCP', '17': 'UDP', udp: 'UDP', '1': 'ICMP', icmp: 'ICMP', '58': 'ICMPv6', icmpv6: 'ICMPv6' }
  return names[rule.protocol] ?? rule.protocol
}
export function formatRulePorts(rule: FirewallRule, labels: Pick<RuleLabels, 'all' | 'unavailable' | 'portList'>, facts?: AliyunRuleFacts): string {
  if (facts?.port_range_list_id || facts?.port_range_list_name) {
    const ref = facts.port_range_list_id && facts.port_range_list_name
      ? `${facts.port_range_list_name} (${facts.port_range_list_id})`
      : facts.port_range_list_name || facts.port_range_list_id
    return `${labels.portList}: ${ref}`
  }
  if (rule.protocol === '-1' || rule.protocol === 'all') return labels.all
  const value = (port: number | undefined) => port === undefined || port === -1 ? labels.all : String(port)
  if (['icmp', 'icmpv6', '1', '58'].includes(rule.protocol)) {
    if (rule.icmp_type === undefined && rule.icmp_code === undefined) return labels.unavailable
    return `${value(rule.icmp_type)} / ${value(rule.icmp_code)}`
  }
  const { from_port: from, to_port: to } = rule
  if (from === undefined && to === undefined) return labels.all
  return from === to ? value(from) : `${value(from)}–${value(to)}`
}
export function formatRulePriority(rule: FirewallRule, unavailable = '—') { return rule.priority == null ? unavailable : String(rule.priority) }
export function formatRuleAction(rule: FirewallRule, labels: Pick<RuleLabels, 'allow' | 'deny'>) { return rule.action === 'allow' ? labels.allow : rule.action === 'deny' ? labels.deny : rule.action }
export function formatRuleEnabled(rule: FirewallRule, labels: Pick<RuleLabels, 'unavailable' | 'enabled' | 'disabled'>) { return rule.enabled == null ? labels.unavailable : rule.enabled ? labels.enabled : labels.disabled }

export function ruleNativeFacts(native: FirewallNativeDetails, direction: 'inbound' | 'outbound', index: number): { aws?: AWSRuleFacts; aliyun?: AliyunRuleFacts } {
  return {
    aws: native[`aws_${direction}_rule_facts`]?.find(fact => fact.rule_index === index),
    aliyun: native[`aliyun_${direction}_rule_facts`]?.find(fact => fact.rule_index === index),
  }
}
export interface RuleTarget { kind: string; values: string[] }
// These are observed selectors, not an evaluation of effective access.
export function ruleTargets(rule: FirewallRule): RuleTarget[] {
  const targets: RuleTarget[] = []
  if (rule.target) targets.push({ kind: rule.target_type, values: [rule.target] })
  for (const kind of ['source_ranges', 'destination_ranges', 'source_network_tags', 'target_network_tags', 'source_service_accounts', 'target_service_accounts'] as const) {
    const values = rule[kind]
    if (values?.length) targets.push({ kind, values })
  }
  return targets
}
