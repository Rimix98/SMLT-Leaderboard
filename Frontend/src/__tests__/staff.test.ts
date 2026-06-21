import { describe, it, expect } from 'vitest'
import { TIER_CONFIG } from '../api/staff'

describe('TIER_CONFIG', () => {
  it('has all tier types', () => {
    expect(TIER_CONFIG).toHaveProperty('priority')
    expect(TIER_CONFIG).toHaveProperty('base')
    expect(TIER_CONFIG).toHaveProperty('reserve')
    expect(TIER_CONFIG).toHaveProperty('na')
  })

  for (const [key, config] of Object.entries(TIER_CONFIG)) {
    it(`${key} has label and color`, () => {
      expect(typeof config.label).toBe('string')
      expect(config.label.length).toBeGreaterThan(0)
      expect(typeof config.color).toBe('string')
      expect(config.color).toMatch(/^#[0-9a-fA-F]{6}$/)
    })
  }
})
