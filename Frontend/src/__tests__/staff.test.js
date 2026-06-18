import { describe, it, expect } from 'vitest'
import { TIER_CONFIG, getNextTier } from '../api/staff.js'

describe('TIER_CONFIG', () => {
  it('has all tier types', () => {
    expect(TIER_CONFIG).toHaveProperty('priority')
    expect(TIER_CONFIG).toHaveProperty('base')
    expect(TIER_CONFIG).toHaveProperty('reserve')
    expect(TIER_CONFIG).toHaveProperty('na')
  })

  each_tier_has_label_and_color()()

  function each_tier_has_label_and_color() {
    return () => {
      for (const [key, config] of Object.entries(TIER_CONFIG)) {
        it(`${key} has label and color`, () => {
          expect(typeof config.label).toBe('string')
          expect(config.label.length).toBeGreaterThan(0)
          expect(typeof config.color).toBe('string')
          expect(config.color).toMatch(/^#[0-9a-fA-F]{6}$/)
        })
      }
    }
  }
})

describe('getNextTier', () => {
  it('cycles na -> priority', () => {
    expect(getNextTier('na')).toBe('priority')
  })

  it('cycles priority -> base', () => {
    expect(getNextTier('priority')).toBe('base')
  })

  it('cycles base -> reserve', () => {
    expect(getNextTier('base')).toBe('reserve')
  })

  it('cycles reserve -> na (wraps around)', () => {
    expect(getNextTier('reserve')).toBe('na')
  })

  it('returns na for unknown tier', () => {
    expect(getNextTier('unknown')).toBe('na')
  })
})
