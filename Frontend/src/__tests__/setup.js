import { vi } from 'vitest'

const mockDocumentElement = {
  _attrs: {},
  setAttribute(k, v) { this._attrs[k] = v },
  getAttribute(k) { return this._attrs[k] || null },
  removeAttribute(k) { delete this._attrs[k] },
}

const mockLocalStorage = {
  _store: {},
  getItem(k) { return this._store[k] || null },
  setItem(k, v) { this._store[k] = String(v) },
  removeItem(k) { delete this._store[k] },
  clear() { this._store = {} },
}

vi.stubGlobal('localStorage', mockLocalStorage)
vi.stubGlobal('document', {
  documentElement: mockDocumentElement,
  body: {
    classList: { add() {}, remove() {}, toggle() {} },
    style: {},
  },
  createElement() { return { style: {}, setAttribute() {}, appendChild() {} } },
  createElementNS() { return { style: {}, setAttribute() {}, appendChild() {} } },
  createTextNode() { return {} },
  createComment() { return {} },
  addEventListener() {},
  removeEventListener() {},
  getElementById() { return null },
  querySelector() { return null },
  head: { appendChild() {} },
})
vi.stubGlobal('setTimeout', vi.fn((fn) => { fn(); return 1 }))
vi.stubGlobal('clearTimeout', vi.fn())
vi.stubGlobal('setInterval', vi.fn())
vi.stubGlobal('clearInterval', vi.fn())
vi.stubGlobal('confirm', vi.fn(() => true))
vi.stubGlobal('fetch', vi.fn())
