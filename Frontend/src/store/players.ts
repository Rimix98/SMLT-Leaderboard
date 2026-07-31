import { reactive } from 'vue'
import type { LeaderboardPlayer, LevelsState } from '../types'

export const playersStore = reactive({
  players: [] as LeaderboardPlayer[],
  allPlayers: [] as LeaderboardPlayer[],
  levels: {
    all: null,
    levelData: null,
    expanded: false,
    filter: '',
    _body: null,
  } as LevelsState,
})

export function resetPlayers(): void {
  playersStore.players = []
  playersStore.allPlayers = []
  playersStore.levels = {
    all: null,
    levelData: null,
    expanded: false,
    filter: '',
    _body: null,
  }
}
