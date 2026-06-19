import { reactive } from 'vue'
import type { Store } from './types'

export const store = reactive<Store>({
  isHost: false,
  theme: localStorage.getItem('smlt-theme') || 'dark',
  players: [],
  allPlayers: [],
  projects: [],
  levels: {
    all: null,
    levelData: null,
    expanded: false,
    filter: '',
    _body: null,
  },
  staffRoles: [],
  staffTiers: [],
  selectedRoleColor: '#3b82f6',
})

let themeTransitionTimer: ReturnType<typeof setTimeout> | null = null

const themes = ['dark', 'light', 'gray'] as const

export function setTheme(theme: string): void {
  if (!(themes as readonly string[]).includes(theme)) return

  document.body.classList.add('theme-transitioning')
  if (themeTransitionTimer) clearTimeout(themeTransitionTimer)
  themeTransitionTimer = setTimeout(() => {
    document.body.classList.remove('theme-transitioning')
  }, 400)

  store.theme = theme
  document.documentElement.setAttribute('data-theme', theme)
  localStorage.setItem('smlt-theme', theme)
}

export function initTheme(): void {
  const saved = (themes as readonly string[]).includes(store.theme) ? store.theme : 'dark'
  store.theme = saved
  document.documentElement.setAttribute('data-theme', saved)
}
