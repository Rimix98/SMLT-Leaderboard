export type Theme = 'dark' | 'light' | 'gray'

export const THEMES: readonly Theme[] = ['dark', 'light', 'gray'] as const

const STORAGE_KEY = 'smlt-theme'

let themeTransitionTimer: ReturnType<typeof setTimeout> | null = null

export interface ThemeStore {
  theme: Theme
}

export const themeStore = reactive<ThemeStore>({
  theme: (typeof localStorage !== 'undefined' && localStorage.getItem(STORAGE_KEY)) || 'dark',
})

export function setTheme(theme: string): void {
  if (!(THEMES as readonly string[]).includes(theme)) return

  if (typeof document !== 'undefined') {
    document.body.classList.add('theme-transitioning')
    if (themeTransitionTimer) clearTimeout(themeTransitionTimer)
    themeTransitionTimer = setTimeout(() => {
      document.body.classList.remove('theme-transitioning')
    }, 400)
  }

  themeStore.theme = theme as Theme
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-theme', theme)
  }
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, theme)
  }
}

export function initTheme(): void {
  const saved: Theme = (THEMES as readonly string[]).includes(themeStore.theme)
    ? themeStore.theme
    : 'dark'
  themeStore.theme = saved
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-theme', saved)
  }
}

import { reactive } from 'vue'
