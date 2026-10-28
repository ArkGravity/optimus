import { describe, expect, it } from 'vitest'
import type { SecurityGroupRule } from '@/types/assets'
import { formatRulePorts, formatRuleProtocol } from '../rules'

const rule = (protocol: string, from_port?: number, to_port?: number): SecurityGroupRule => ({ protocol, from_port, to_port, target_type: 'ipv4', target: '0.0.0.0/0', description: '' })

describe('security group rule display', () => {
  it('handles all protocols, ranges and port zero', () => {
    expect(formatRuleProtocol(rule('-1'), 'All')).toBe('All')
    expect(formatRuleProtocol(rule('6'), 'All')).toBe('TCP')
    expect(formatRuleProtocol(rule('47'), 'All')).toBe('47')
    expect(formatRulePorts(rule('-1', 0, 0), 'All')).toBe('All')
    expect(formatRulePorts(rule('tcp', 0, 0), 'All')).toBe('0')
    expect(formatRulePorts(rule('tcp', 80, 443), 'All')).toBe('80–443')
    expect(formatRulePorts(rule('47'), 'All')).toBe('All')
  })
  it('preserves ICMP type and code zero and wildcard values', () => {
    expect(formatRulePorts(rule('icmp', 8, 0), 'All')).toBe('8 / 0')
    expect(formatRulePorts(rule('58', -1, -1), 'All')).toBe('All / All')
  })
})
