import { reactive } from 'vue'
import { themeStore, setTheme, initTheme } from './theme'
import { authStore, setHost } from './auth'
import { playersStore, resetPlayers } from './players'
import { smpStore } from './smp'
import type { Store } from '../types'

export { themeStore, setTheme, initTheme }
export { authStore, setHost }
export { playersStore, resetPlayers }
export { smpStore }

export function resetAll(): void {
  authStore.isHost = false
  resetPlayers()
  smpStore.status = null
}

export const store = reactive<Store>({
  get isHost() {
    return authStore.isHost
  },
  set isHost(v: boolean) {
    authStore.isHost = v
  },
  get theme() {
    return themeStore.theme
  },
  set theme(v: string) {
    themeStore.theme = v as typeof themeStore.theme
  },
  get players() {
    return playersStore.players
  },
  set players(v: Store['players']) {
    playersStore.players = v
  },
  get allPlayers() {
    return playersStore.allPlayers
  },
  set allPlayers(v: Store['allPlayers']) {
    playersStore.allPlayers = v
  },
  get levels() {
    return playersStore.levels
  },
  set levels(v: Store['levels']) {
    playersStore.levels = v
  },
})
