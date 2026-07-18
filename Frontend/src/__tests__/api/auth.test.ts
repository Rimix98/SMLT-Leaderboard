import { describe, it, expect, vi, beforeEach } from 'vitest'
import { store } from '../../store'

// Mock fetch globally
const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

// Mock showToast
vi.mock('../../api/utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api/utils')>()
  return {
    ...actual,
    showToast: vi.fn(),
  }
})

describe('Auth flow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    store.isHost = false
    mockFetch.mockReset()
  })

  describe('store.isHost', () => {
    it('defaults to false', () => {
      expect(store.isHost).toBe(false)
    })

    it('can be set to true', () => {
      store.isHost = true
      expect(store.isHost).toBe(true)
    })

    it('is reactive', () => {
      const values: boolean[] = []
      // Simple reactivity test
      store.isHost = false
      values.push(store.isHost)
      store.isHost = true
      values.push(store.isHost)
      expect(values).toEqual([false, true])
    })
  })

  describe('CSRF token flow', () => {
    it('tokens object has expected shape', async () => {
      const { tokens } = await import('../../api/utils')
      expect(tokens).toHaveProperty('csrfToken')
      expect(tokens).toHaveProperty('adminKnockKey')
      expect(typeof tokens.csrfToken).toBe('string')
      expect(typeof tokens.adminKnockKey).toBe('string')
    })
  })

  describe('isAbortError', () => {
    it('identifies AbortError', async () => {
      const { isAbortError } = await import('../../api/utils')
      const err = new DOMException('aborted', 'AbortError')
      expect(isAbortError(err)).toBe(true)
    })

    it('rejects non-AbortError', async () => {
      const { isAbortError } = await import('../../api/utils')
      expect(isAbortError(new Error('test'))).toBe(false)
      expect(isAbortError(null)).toBe(false)
      expect(isAbortError(undefined)).toBe(false)
    })
  })

  describe('parseJsonResponse', () => {
    it('parses valid JSON', async () => {
      const { parseJsonResponse } = await import('../../api/utils')
      const res = new Response('{"ok":true}', {
        headers: { 'Content-Type': 'application/json' }
      })
      const data = await parseJsonResponse(res)
      expect(data.ok).toBe(true)
    })

    it('returns empty object for empty body', async () => {
      const { parseJsonResponse } = await import('../../api/utils')
      const res = new Response('', {
        headers: { 'Content-Type': 'application/json' }
      })
      const data = await parseJsonResponse(res)
      expect(data).toEqual({})
    })

    it('throws for HTML response', async () => {
      const { parseJsonResponse } = await import('../../api/utils')
      const res = new Response('<html><body>Error</body></html>', {
        headers: { 'Content-Type': 'text/html' }
      })
      await expect(parseJsonResponse(res)).rejects.toThrow()
    })
  })
})
