import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock fetch globally before importing anything
const mockFetch = vi.fn()
vi.stubGlobal('fetch', mockFetch)

import { filterPlayers, getFilteredLevels, expandLevels, filterLevels } from '../../api/leaderboard'
import { store } from '../../store'

describe('Leaderboard API module', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    store.players = []
    store.allPlayers = []
    store.levels = { all: null, levelData: null, expanded: false, filter: '', _body: null }
  })

  describe('filterPlayers', () => {
    beforeEach(() => {
      store.allPlayers = [
        { id: 1, name: 'Alice', rank: 1, score: 100, nationality: 'RU', records: [], hardest: null },
        { id: 2, name: 'Bob', rank: 2, score: 80, nationality: 'US', records: [], hardest: null },
        { id: 3, name: 'Charlie', rank: 3, score: 60, nationality: 'DE', records: [], hardest: null },
      ]
    })

    it('shows all players when query is empty', () => {
      filterPlayers('')
      expect(store.players).toHaveLength(3)
    })

    it('filters by name case-insensitively', () => {
      filterPlayers('alice')
      expect(store.players).toHaveLength(1)
      expect(store.players[0].name).toBe('Alice')
    })

    it('filters partial matches', () => {
      filterPlayers('ob')
      expect(store.players).toHaveLength(1)
      expect(store.players[0].name).toBe('Bob')
    })

    it('returns empty for no matches', () => {
      filterPlayers('zzz')
      expect(store.players).toHaveLength(0)
    })
  })

  describe('getFilteredLevels', () => {
    it('returns empty when no levels', () => {
      store.levels.all = null
      expect(getFilteredLevels()).toEqual([])
    })

    it('returns all levels when expanded', () => {
      store.levels.all = Array.from({ length: 50 }, (_, i) => ({
        id: i, name: `Level ${i}`, placement: i + 1, victors: []
      }))
      store.levels.expanded = true
      store.levels.filter = ''
      const result = getFilteredLevels()
      expect(result).toHaveLength(50)
    })

    it('limits to 39 when not expanded', () => {
      store.levels.all = Array.from({ length: 50 }, (_, i) => ({
        id: i, name: `Level ${i}`, placement: i + 1, victors: []
      }))
      store.levels.expanded = false
      store.levels.filter = ''
      const result = getFilteredLevels()
      expect(result).toHaveLength(39)
    })

    it('filters by level name', () => {
      store.levels.all = [
        { id: 1, name: 'Bloodbath', placement: 1, victors: [] },
        { id: 2, name: 'Tartarus', placement: 2, victors: [] },
        { id: 3, name: 'Slaughterhouse', placement: 3, victors: [] },
      ]
      store.levels.filter = 'blood'
      const result = getFilteredLevels()
      expect(result).toHaveLength(1)
      expect(result[0].name).toBe('Bloodbath')
    })
  })

  describe('expandLevels', () => {
    it('toggles expanded state', () => {
      store.levels.expanded = false
      expandLevels()
      expect(store.levels.expanded).toBe(true)
      expandLevels()
      expect(store.levels.expanded).toBe(false)
    })
  })

  describe('filterLevels', () => {
    it('sets the filter string', () => {
      store.levels.filter = ''
      filterLevels('test')
      expect(store.levels.filter).toBe('test')
    })
  })
})
