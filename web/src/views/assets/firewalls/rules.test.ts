import { describe, expect, it } from 'vitest'
import type { FirewallRule, FirewallNativeDetails } from '@/types/assets'
import { formatRuleProtocol, formatRulePorts, formatRuleAction, formatRulePriority, formatRuleEnabled, ruleNativeFacts, ruleTargets } from './rules'

const rule = (patch: Partial<FirewallRule> = {}): FirewallRule => ({ direction: 'inbound', protocol: 'tcp', target_type: '', target: '', description: '', action: 'allow', priority: null, enabled: null, source_ranges: [], destination_ranges: [], source_network_tags: [], target_network_tags: [], source_service_accounts: [], target_service_accounts: [], ...patch })
const labels = { all: 'All', unavailable: 'N/A', allow: 'Allow', deny: 'Deny', enabled: 'Enabled', disabled: 'Disabled', portList: 'Port list' }
describe('common firewall formatting', () => {
  it('retains AWS protocol, port zero, ranges and explicit ICMP type/code', () => {
    expect(formatRuleProtocol(rule({ protocol: '-1' }), labels.all)).toBe('All')
    expect(formatRuleProtocol(rule({ protocol: '6' }), labels.all)).toBe('TCP')
    expect(formatRuleProtocol(rule({ protocol: '47' }), labels.all)).toBe('47')
    expect(formatRulePorts(rule({ from_port: 0, to_port: 0 }), labels)).toBe('0')
    expect(formatRulePorts(rule({ from_port: 80, to_port: 443 }), labels)).toBe('80–443')
    expect(formatRulePorts(rule({ protocol: 'icmp', icmp_type: 8, icmp_code: 0 }), labels)).toBe('8 / 0')
    expect(formatRulePorts(rule({ protocol: '58', icmp_type: -1, icmp_code: -1 }), labels)).toBe('All / All')
  })
  it('displays Google all protocol and empty TCP/UDP ports without fabricating ICMP fields', () => {
    expect(formatRuleProtocol(rule({ protocol: 'all' }), labels.all)).toBe('All')
    expect(formatRulePorts(rule(), labels)).toBe('All')
    expect(formatRulePorts(rule({ protocol: 'udp' }), labels)).toBe('All')
    expect(formatRulePorts(rule({ protocol: 'icmp' }), labels)).toBe('N/A')
    expect(formatRulePorts(rule({ protocol: 'icmp', from_port: 8, to_port: 0 }), labels)).toBe('N/A')
  })
  it('distinguishes null priority/enabled from observed zero/false and formats allow/deny', () => {
    expect(formatRulePriority(rule())).toBe('—')
    expect(formatRulePriority(rule({ priority: 0 }))).toBe('0')
    expect(formatRuleEnabled(rule(), labels)).toBe('N/A')
    expect(formatRuleEnabled(rule({ enabled: false }), labels)).toBe('Disabled')
    expect(formatRuleEnabled(rule({ enabled: true }), labels)).toBe('Enabled')
    expect(formatRuleAction(rule(), labels)).toBe('Allow')
    expect(formatRuleAction(rule({ action: 'deny' }), labels)).toBe('Deny')
  })
  it('joins indexed native metadata by direction/index and preserves managed destination port lists', () => {
    const native: FirewallNativeDetails = {
      aliyun_inbound_rule_facts: [{ rule_index: 2, port_range_list_id: 'pl-2', port_range_list_name: 'Web', source_port_range: '100/200' }],
      aliyun_outbound_rule_facts: [{ rule_index: 2, source_port_range: '300/400' }],
      aws_inbound_rule_facts: [{ rule_index: 0, owner_id: 'owner', vpc_id: 'vpc', peering_id: 'peer' }],
    }
    const facts = ruleNativeFacts(native, 'inbound', 2)
    expect(facts.aliyun?.port_range_list_id).toBe('pl-2')
    expect(ruleNativeFacts(native, 'outbound', 2).aliyun?.source_port_range).toBe('300/400')
    expect(ruleNativeFacts(native, 'inbound', 0).aws?.peering_id).toBe('peer')
    expect(formatRulePorts(rule(), labels, facts.aliyun)).toBe('Port list: Web (pl-2)')
    expect(formatRulePorts(rule(), labels, { rule_index: 0, port_range_list_name: 'Managed' })).toBe('Port list: Managed')
    expect(formatRulePorts(rule({ from_port: 443, to_port: 443 }), labels, { rule_index: 0, source_port_range: '100/200' })).toBe('443')
  })
  it('retains every typed target including ranges, network tags and service accounts', () => {
    for (const target_type of ['ipv4', 'ipv6', 'security_group', 'prefix_list']) {
      expect(ruleTargets(rule({ target_type, target: 'native-ref' }))).toContainEqual({ kind: target_type, values: ['native-ref'] })
    }
    expect(ruleTargets(rule({ source_ranges: ['10.0.0.0/8'], destination_ranges: ['2001:db8::/32'], source_network_tags: ['web'], target_network_tags: ['db'], source_service_accounts: ['source@example.com'], target_service_accounts: ['target@example.com'] }))).toEqual([
      { kind: 'source_ranges', values: ['10.0.0.0/8'] }, { kind: 'destination_ranges', values: ['2001:db8::/32'] },
      { kind: 'source_network_tags', values: ['web'] }, { kind: 'target_network_tags', values: ['db'] },
      { kind: 'source_service_accounts', values: ['source@example.com'] }, { kind: 'target_service_accounts', values: ['target@example.com'] },
    ])
  })
})
