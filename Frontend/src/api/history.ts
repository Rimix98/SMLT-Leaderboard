import { fetchWithAbort, parseJsonResponse, isAbortError, BACKEND_URL, tokens, doAdminKnock, showToast } from './utils'

export interface PlayerHistoryEntry {
  playerId: string
  playerName: string
  date: string
  rank: number
  score: number
  recordsCount: number
  hardestLevel: string
  snapshotAt: string
}

export interface LeaderboardCheckResponse {
  hash: string
  lastUpdated: string
  playerCount: number
}

let _lastLeaderboardHash = ''

export function getLastLeaderboardHash(): string {
  return _lastLeaderboardHash
}

export function setLastLeaderboardHash(hash: string): void {
  _lastLeaderboardHash = hash
}

export async function checkLeaderboardChanged(): Promise<boolean> {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/leaderboard/check`, {}, 'leaderboard-check')
    if (!res.ok) return false
    const data = await parseJsonResponse(res) as unknown as LeaderboardCheckResponse
    if (data.hash && data.hash !== _lastLeaderboardHash) {
      _lastLeaderboardHash = data.hash
      return true
    }
    return false
  } catch {
    return false
  }
}

export async function getPlayerHistory(playerId: string): Promise<PlayerHistoryEntry[]> {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/history/${encodeURIComponent(playerId)}`, {}, `history-${playerId}`)
    if (!res.ok) return []
    const data = await res.json()
    return Array.isArray(data) ? data : []
  } catch {
    return []
  }
}

export async function saveHistorySnapshot(): Promise<boolean> {
  try {
    if (!tokens.adminKnockKey) await doAdminKnock()
    const res = await fetchWithAbort(`${BACKEND_URL}/history/snapshot`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
    }, 'history-snapshot')
    if (!res.ok) {
      const err = await parseJsonResponse(res)
      throw new Error((err.error as string) || 'Ошибка сохранения снимка')
    }
    showToast('Снимок истории сохранён', 'success')
    return true
  } catch (e) {
    if (!isAbortError(e)) showToast((e as Error).message, 'error')
    return false
  }
}
