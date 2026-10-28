import type { SecurityGroupRule } from '@/types/assets'

export function formatRuleProtocol(rule: SecurityGroupRule, all: string): string {
  const names: Record<string, string> = { '-1': all, '6': 'TCP', tcp: 'TCP', '17': 'UDP', udp: 'UDP', '1': 'ICMP', icmp: 'ICMP', '58': 'ICMPv6', icmpv6: 'ICMPv6' }
  return names[rule.protocol] ?? rule.protocol
}

export function formatRulePorts(rule: SecurityGroupRule, all: string): string {
  if (rule.protocol === '-1') return all
  const { from_port: from, to_port: to } = rule
  const value = (port: number | undefined) => port === undefined || port === -1 ? all : String(port)
  if (['icmp', 'icmpv6', '1', '58'].includes(rule.protocol)) return `${value(from)} / ${value(to)}`
  if (from === undefined && to === undefined) return all
  return from === to ? value(from) : `${value(from)}–${value(to)}`
}
