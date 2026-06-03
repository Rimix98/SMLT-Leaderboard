import { reactive } from 'vue'

export const store = reactive({
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

let themeTransitionTimer = null

const themes = ['dark', 'light', 'gray']

export function setTheme(theme) {
  if (!themes.includes(theme)) return

  document.body.classList.add('theme-transitioning')
  if (themeTransitionTimer) clearTimeout(themeTransitionTimer)
  themeTransitionTimer = setTimeout(() => {
    document.body.classList.remove('theme-transitioning')
  }, 400)

  store.theme = theme
  document.documentElement.setAttribute('data-theme', theme)
  localStorage.setItem('smlt-theme', theme)
}

export function initTheme() {
  const saved = themes.includes(store.theme) ? store.theme : 'dark'
  store.theme = saved
  document.documentElement.setAttribute('data-theme', saved)
}
