import { vi } from 'vitest'

// Only stub DOM globals when running outside happy-dom (node environment).
// happy-dom provides real window/document/SVGElement for component tests.
const needsDOMStubs = typeof window === 'undefined'

if (needsDOMStubs) {
  const mockDocumentElement = {
    _attrs: {} as Record<string, string>,
    setAttribute(k: string, v: string) { this._attrs[k] = v },
    getAttribute(k: string) { return this._attrs[k] || null },
    removeAttribute(k: string) { delete this._attrs[k] },
  }

  const mockLocalStorage = {
    _store: {} as Record<string, string>,
    getItem(k: string) { return this._store[k] || null },
    setItem(k: string, v: string) { this._store[k] = String(v) },
    removeItem(k: string) { delete this._store[k] },
    clear() { this._store = {} },
  }

  vi.stubGlobal('localStorage', mockLocalStorage)
  vi.stubGlobal('document', {
    documentElement: mockDocumentElement,
    body: {
      classList: { add() {}, remove() {}, toggle() {} },
      style: {},
    },
    createElement() { return { style: {} as Record<string, string>, setAttribute() {}, appendChild() {} } },
    createElementNS() { return { style: {} as Record<string, string>, setAttribute() {}, appendChild() {} } },
    createTextNode() { return {} },
    createComment() { return {} },
    addEventListener() {},
    removeEventListener() {},
    getElementById() { return null },
    querySelector() { return null },
    head: { appendChild() {} },
  })
  vi.stubGlobal('window', {
    document: {},
    localStorage: {},
    addEventListener() {},
    removeEventListener() {},
    location: { href: '', origin: '', protocol: 'https:' },
    navigator: { userAgent: 'test' },
  })
}

vi.stubGlobal('setTimeout', vi.fn((fn: () => void) => { fn(); return 1 }))
vi.stubGlobal('clearTimeout', vi.fn())
vi.stubGlobal('setInterval', vi.fn())
vi.stubGlobal('clearInterval', vi.fn())
vi.stubGlobal('confirm', vi.fn(() => true))
vi.stubGlobal('fetch', vi.fn())
