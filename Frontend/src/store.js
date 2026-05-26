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
  pendingProjectParticipants: [],
  _selectedParticipant: '',
})

let themeTransitionTimer = null

export function toggleTheme() {
  const current = store.theme
  const next = current === 'dark' ? 'light' : 'dark'

  document.body.classList.add('theme-transitioning')
  if (themeTransitionTimer) clearTimeout(themeTransitionTimer)
  themeTransitionTimer = setTimeout(() => {
    document.body.classList.remove('theme-transitioning')
  }, 400)

  store.theme = next
  document.documentElement.setAttribute('data-theme', next)
  localStorage.setItem('smlt-theme', next)
}

export function initTheme() {
  const saved = store.theme
  document.documentElement.setAttribute('data-theme', saved)
}
