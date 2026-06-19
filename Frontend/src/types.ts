export interface StaffPlayer {
  nickname: string
  discord?: string
}

export interface StaffRole {
  name: string
  color: string
  players: StaffPlayer[]
  tiersEnabled?: boolean
}

export interface StaffTierEntry {
  nickname: string
  tier: TierKey
}

export type TierKey = 'priority' | 'base' | 'reserve' | 'na'

export interface TierConfig {
  label: string
  color: string
}

export interface ParticipantPart {
  gp: string[]
  deco: string[]
  transition: string
}

export interface ParticipantConfig {
  host: string
  parts: ParticipantPart[]
  endScreen: string[]
  playtest: string[]
  verifier: string[]
  merger: string[]
  merger2: string[]
  showcaser: string
  fxMode: boolean
  soloGp: string | null
}

export interface Project {
  name: string
  videoId: string
  id: string
  comment: string
  status: string
  verifier: string
  participants: string[]
}

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
  projects: Project[]
  levels: LevelsState
  staffRoles: StaffRole[]
  staffTiers: StaffTierEntry[]
  selectedRoleColor: string
}
