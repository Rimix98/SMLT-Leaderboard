import { describe, it, expect, vi, beforeEach } from 'vitest'

const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

import { getLastLeaderboardHash, setLastLeaderboardHash, checkLeaderboardChanged } from '../../api/history'

describe('History API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setLastLeaderboardHash('')
    mockFetch.mockReset()
  })

  describe('getLastLeaderboardHash / setLastLeaderboardHash', () => {
    it('defaults to empty string', () => {
      expect(getLastLeaderboardHash()).toBe('')
    })

    it('stores and retrieves hash', () => {
      setLastLeaderboardHash('abc123')
      expect(getLastLeaderboardHash()).toBe('abc123')
    })

    it('overwrites previous hash', () => {
      setLastLeaderboardHash('first')
      setLastLeaderboardHash('second')
      expect(getLastLeaderboardHash()).toBe('second')
    })
  })

  describe('checkLeaderboardChanged', () => {
    it('returns false on fetch error', async () => {
      mockFetch.mockRejectedValue(new Error('network'))
      const result = await checkLeaderboardChanged()
      expect(result).toBe(false)
    })

    it('returns false when response is not ok', async () => {
      mockFetch.mockResolvedValue(new Response(null, { status: 500 }))
      const result = await checkLeaderboardChanged()
      expect(result).toBe(false)
    })

    it('returns true when hash changes', async () => {
      const body = JSON.stringify({ hash: 'newhash', lastUpdated: '', playerCount: 0 })
      mockFetch.mockResolvedValue(new Response(body, {
        headers: { 'Content-Type': 'application/json' },
        status: 200,
      }))
      const result = await checkLeaderboardChanged()
      expect(result).toBe(true)
      expect(getLastLeaderboardHash()).toBe('newhash')
    })

    it('returns false when hash stays the same', async () => {
      setLastLeaderboardHash('samehash')
      const body = JSON.stringify({ hash: 'samehash', lastUpdated: '', playerCount: 0 })
      mockFetch.mockResolvedValue(new Response(body, {
        headers: { 'Content-Type': 'application/json' },
        status: 200,
      }))
      const result = await checkLeaderboardChanged()
      expect(result).toBe(false)
    })
  })
})
