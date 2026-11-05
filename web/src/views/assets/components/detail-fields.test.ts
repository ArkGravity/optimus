import { describe, expect, it } from 'vitest'
import { parseDetailKey } from './detail-fields'
describe('asset detail identities', () => {
  it('keeps the backend int64 range exact for canonical and legacy string identities', () => {
    expect(parseDetailKey('gcp:instance:9223372036854775807', 'instance')).toBe('gcp:instance:9223372036854775807')
    expect(parseDetailKey('9223372036854775807', 'instance')).toBe('aws:instance:9223372036854775807')
    expect(parseDetailKey('9007199254740993', 'vpc')).toBe('aws:vpc:9007199254740993')
    expect(parseDetailKey('42', 'database')).toBe(42)
    expect(parseDetailKey(42, 'firewall')).toBe(42)
  })
  it('rejects overflowing, rounded, mismatched and noncanonical identities', () => {
    for (const raw of ['9223372036854775808', 'aws:instance:9223372036854775808', 'gcp:vpc:42', 'aws:instance:042', '042', '0', '-1', 'AWS:instance:42', 'aws:instance:1e3', Number.MAX_SAFE_INTEGER + 2, NaN, undefined]) {
      expect(parseDetailKey(raw, 'instance'), String(raw)).toBeUndefined()
    }
  })
})
