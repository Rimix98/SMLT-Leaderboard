export interface LevelRecord {
  status: string
  percent?: number
  progress?: number
  level?: {
    id: number
    name: string
    placement: number
  }
}

export interface LeaderboardPlayer {
  id: number | string
  name: string
  rank: number
  score: number
  nationality: string | null
  records: LevelRecord[]
  hardest: LevelRecord | null
}

export interface LevelData {
  id: number
  name: string
  placement: number
  victors: { id: number | string; name: string; nationality: string | null }[]
}

export interface LevelsState {
  all: LevelData[] | null
  levelData: Map<string, LevelData> | null
  expanded: boolean
  filter: string
  _body: unknown
}

export interface Store {
  isHost: boolean
  theme: string
  players: LeaderboardPlayer[]
  allPlayers: LeaderboardPlayer[]
  levels: LevelsState
}

export interface SMPStatus {
  online: boolean
  playersMax: number
  playersOnline: number
  version: string
  serverIp: string
  fetchedAt: string
}

export type LocaleCode = 'ru' | 'en'
