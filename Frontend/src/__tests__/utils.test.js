import { describe, it, expect } from 'vitest'
import {
  API_BASE,
  BACKEND_URL,
  tokens,
  isAbortError,
  resolveCountry,
  CODE_TO_NAME,
  getFlagCode,
} from '../api/utils.js'

describe('API constants', () => {
  it('API_BASE is demonlist.org', () => {
    expect(API_BASE).toBe('https://api.demonlist.org')
  })

  it('BACKEND_URL is /api', () => {
    expect(BACKEND_URL).toBe('/api')
  })
})

describe('tokens', () => {
  it('has csrfToken and adminKnockKey', () => {
    expect(tokens).toHaveProperty('csrfToken')
    expect(tokens).toHaveProperty('adminKnockKey')
  })
})

describe('isAbortError', () => {
  it('returns true for AbortError', () => {
    const err = new DOMException('The operation was aborted', 'AbortError')
    expect(isAbortError(err)).toBe(true)
  })

  it('returns false for other errors', () => {
    const err = new Error('something else')
    expect(isAbortError(err)).toBe(false)
  })

  it('returns false for null', () => {
    expect(isAbortError(null)).toBe(false)
  })
})

describe('resolveCountry', () => {
  it('resolves known country names', () => {
    const result = resolveCountry('Russia')
    expect(result).toBeTruthy()
  })

  it('returns null for unknown country', () => {
    const result = resolveCountry('Narnia')
    expect(result).toBeNull()
  })

  it('is case-insensitive', () => {
    const lower = resolveCountry('russia')
    const upper = resolveCountry('RUSSIA')
    expect(lower).toBe(upper)
  })
})

describe('CODE_TO_NAME', () => {
  it('contains common country codes', () => {
    expect(CODE_TO_NAME).toHaveProperty('RU')
    expect(CODE_TO_NAME).toHaveProperty('US')
    expect(CODE_TO_NAME).toHaveProperty('DE')
  })

  it('values are strings', () => {
    for (const [key, val] of Object.entries(CODE_TO_NAME)) {
      expect(typeof val).toBe('string')
      expect(val.length).toBeGreaterThan(0)
    }
  })
})

describe('getFlagCode', () => {
  it('returns a string for known country', () => {
    const code = getFlagCode('Russia')
    expect(typeof code).toBe('string')
  })
})
