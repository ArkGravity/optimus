import { describe, expect, it } from 'vitest'
import en from '@/locales/en-US.json'
import zh from '@/locales/zh-CN.json'
import { scalarFact } from './use-asset-list'

describe('resource scalar unavailable presentation', () => {
  for (const [locale, messages] of [['en-US', en], ['zh-CN', zh]] as const) {
    it(`localizes missing scalar facts and preserves real zero and false in ${locale}`, () => {
      const t = (key: string) => key === 'assets.detail.not_applicable' ? messages.assets.detail.not_applicable : key
      for (const value of [undefined, null, '']) expect(scalarFact(value, t)).toBe(messages.assets.detail.not_applicable)
      for (const value of [0, false, 'READY']) expect(scalarFact(value, t)).toBe(value)
    })
  }
})
