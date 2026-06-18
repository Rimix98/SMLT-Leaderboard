import { describe, it, expect, beforeEach } from 'vitest'
import { store, setTheme, initTheme } from '../store.js'

describe('store', () => {
  beforeEach(() => {
    store.isHost = false
    store.players = []
    store.allPlayers = []
    store.projects = []
    store.levels = { all: null, levelData: null, expanded: false, filter: '', _body: null }
    store.staffRoles = []
    store.staffTiers = []
  })

  it('has correct default values', () => {
    expect(store.isHost).toBe(false)
    expect(store.players).toEqual([])
    expect(store.allPlayers).toEqual([])
    expect(store.projects).toEqual([])
    expect(store.staffRoles).toEqual([])
    expect(store.staffTiers).toEqual([])
  })

  it('defaults theme to dark', () => {
    expect(store.theme).toBe('dark')
  })

  it('is reactive', () => {
    store.isHost = true
    expect(store.isHost).toBe(true)
    store.players = [{ name: 'test' }]
    expect(store.players).toHaveLength(1)
  })
})

describe('setTheme', () => {
  beforeEach(() => {
    document.documentElement.removeAttribute('data-theme')
  })

  it('sets valid theme', () => {
    setTheme('light')
    expect(store.theme).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
  })

  it('rejects invalid theme', () => {
    store.theme = 'dark'
    setTheme('invalid')
    expect(store.theme).toBe('dark')
  })

  it('sets dark theme', () => {
    setTheme('dark')
    expect(store.theme).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })

  it('sets gray theme', () => {
    setTheme('gray')
    expect(store.theme).toBe('gray')
    expect(document.documentElement.getAttribute('data-theme')).toBe('gray')
  })
})

describe('initTheme', () => {
  it('sets theme attribute on document', () => {
    store.theme = 'light'
    initTheme()
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
  })

  it('defaults to dark for invalid theme', () => {
    store.theme = 'invalid'
    initTheme()
    expect(store.theme).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
  })
})
