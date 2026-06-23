interface VaWindow {
  va?: ((...args: unknown[]) => void) & { q?: unknown[][] }
  vaq?: unknown[][]
}

const w = window as VaWindow
w.va = w.va || function (...args: unknown[]) { (w.vaq = w.vaq || []).push(args) }
