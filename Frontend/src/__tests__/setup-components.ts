import { vi } from 'vitest'

// Minimal stubs needed for non-DOM tests that run alongside component tests
vi.stubGlobal('setTimeout', vi.fn((fn: () => void) => { fn(); return 1 }))
vi.stubGlobal('clearTimeout', vi.fn())
vi.stubGlobal('setInterval', vi.fn())
vi.stubGlobal('clearInterval', vi.fn())
vi.stubGlobal('confirm', vi.fn(() => true))
