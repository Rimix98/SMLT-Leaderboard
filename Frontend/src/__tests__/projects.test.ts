import { describe, it, expect } from 'vitest'
import {
  getStatusClass,
  extractVideoId,
  toYoutubeId11,
  generateProjectId,
  createDefaultParticipantConfig,
  parseParticipantConfig,
  serializeParticipantConfig,
} from '../api/projects'
import { store } from '../store'

describe('getStatusClass', () => {
  it('returns correct class for готов', () => {
    expect(getStatusClass('готов')).toBe('status-ready')
  })

  it('returns correct class for в процессе верифа', () => {
    expect(getStatusClass('в процессе верифа')).toBe('status-verifying')
  })

  it('returns correct class for в процессе постройки', () => {
    expect(getStatusClass('в процессе постройки')).toBe('status-building')
  })

  it('returns correct class for планируется', () => {
    expect(getStatusClass('планируется')).toBe('status-planned')
  })

  it('returns correct class for заморожен', () => {
    expect(getStatusClass('заморожен')).toBe('status-frozen')
  })

  it('returns correct class for мёртв', () => {
    expect(getStatusClass('мёртв')).toBe('status-dead')
  })

  it('returns default for unknown status', () => {
    expect(getStatusClass('something')).toBe('status-planned')
  })

  it('returns default for empty string', () => {
    expect(getStatusClass('')).toBe('status-planned')
  })

  it('is case-insensitive', () => {
    expect(getStatusClass('ГОТОВ')).toBe('status-ready')
  })
})

describe('extractVideoId', () => {
  it('extracts from youtube.com/watch', () => {
    expect(extractVideoId('https://www.youtube.com/watch?v=dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ')
  })

  it('extracts from youtu.be', () => {
    expect(extractVideoId('https://youtu.be/dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ')
  })

  it('extracts from embed URL', () => {
    expect(extractVideoId('https://www.youtube.com/embed/dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ')
  })

  it('extracts from shorts URL', () => {
    expect(extractVideoId('https://www.youtube.com/shorts/dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ')
  })

  it('extracts bare 11-char ID', () => {
    expect(extractVideoId('dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ')
  })

  it('returns empty for invalid URL', () => {
    expect(extractVideoId('https://example.com')).toBe('')
  })

  it('returns empty for empty string', () => {
    expect(extractVideoId('')).toBe('')
  })

  it('returns empty for short string', () => {
    expect(extractVideoId('abc')).toBe('')
  })
})

describe('toYoutubeId11', () => {
  it('returns valid 11-char ID', () => {
    expect(toYoutubeId11('dQw4w9WgXcQ')).toBe('dQw4w9WgXcQ')
  })

  it('returns null for invalid ID', () => {
    expect(toYoutubeId11('short')).toBeNull()
  })

  it('returns null for empty', () => {
    expect(toYoutubeId11('')).toBeNull()
  })

  it('returns null for null/undefined', () => {
    expect(toYoutubeId11(null as unknown as string)).toBeNull()
    expect(toYoutubeId11(undefined as unknown as string)).toBeNull()
  })
})

describe('generateProjectId', () => {
  it('returns string starting with proj_', () => {
    const id = generateProjectId()
    expect(id).toMatch(/^proj_/)
  })

  it('generates unique IDs', () => {
    const ids = new Set<string>()
    for (let i = 0; i < 100; i++) {
      ids.add(generateProjectId())
    }
    expect(ids.size).toBe(100)
  })
})

describe('createDefaultParticipantConfig', () => {
  it('returns object with all expected fields', () => {
    const config = createDefaultParticipantConfig()
    expect(config).toHaveProperty('host', '')
    expect(config).toHaveProperty('parts', [])
    expect(config).toHaveProperty('endScreen', [])
    expect(config).toHaveProperty('playtest', [])
    expect(config).toHaveProperty('verifier', [])
    expect(config).toHaveProperty('merger', [])
    expect(config).toHaveProperty('merger2', [])
    expect(config).toHaveProperty('showcaser', '')
    expect(config).toHaveProperty('fxMode', false)
    expect(config).toHaveProperty('soloGp', null)
  })
})

describe('parseParticipantConfig', () => {
  it('returns default config for null project', () => {
    const config = parseParticipantConfig(null as unknown as { participants?: string[] })
    expect(config.host).toBe('')
    expect(config.parts).toEqual([])
  })

  it('returns default config for empty participants', () => {
    const config = parseParticipantConfig({ participants: [] })
    expect(config.host).toBe('')
  })

  it('parses JSON format', () => {
    const project = {
      participants: [JSON.stringify({ host: 'Player1', parts: [{ gp: ['Player2'], deco: ['Player3'], transition: '' }] })],
    }
    const config = parseParticipantConfig(project)
    expect(config.host).toBe('Player1')
    expect(config.parts).toHaveLength(1)
    expect(config.parts[0].gp).toEqual(['Player2'])
  })

  it('parses old format with dash', () => {
    const project = {
      participants: ['Player1 - GP DECO'],
    }
    const config = parseParticipantConfig(project)
    expect(config.parts).toHaveLength(1)
    expect(config.parts[0].gp).toContain('Player1')
  })

  it('parses old format with parentheses', () => {
    const project = {
      participants: ['Player1 (HOST)'],
    }
    const config = parseParticipantConfig(project)
    expect(config.host).toBe('Player1')
  })

  it('parses HOST role correctly', () => {
    const project = {
      participants: ['Alice - HOST'],
    }
    const config = parseParticipantConfig(project)
    expect(config.host).toBe('Alice')
  })

  it('parses VERIFIER role correctly', () => {
    const project = {
      participants: ['Bob - VERIFIER'],
    }
    const config = parseParticipantConfig(project)
    expect(config.verifier).toContain('Bob')
  })

  it('parses SHOWCASER role correctly', () => {
    const project = {
      participants: ['Charlie - SHOWCASER'],
    }
    const config = parseParticipantConfig(project)
    expect(config.showcaser).toBe('Charlie')
  })
})

describe('serializeParticipantConfig', () => {
  it('serializes config to array with JSON string', () => {
    const config = { host: 'Test', parts: [] }
    const result = serializeParticipantConfig(config as import('../types').ParticipantConfig)
    expect(Array.isArray(result)).toBe(true)
    expect(result).toHaveLength(1)
    expect(JSON.parse(result[0])).toEqual(config)
  })
})
