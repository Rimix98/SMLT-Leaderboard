import { fetchWithAbort, parseJsonResponse, isAbortError, BACKEND_URL, showToast, tokens, doAdminKnock } from './utils'

export interface ShameBoardEntry {
  discordId: string
  username: string
  avatar: string
  reason: string
  addedAt: string
  addedBy: string
}

export interface ShameCheckResult {
  newMembers: { discordId: string; username: string }[]
  totalOnBoard: number
}

let cachedEntries: ShameBoardEntry[] | null = null
let cacheTime = 0

export async function loadShameBoard(): Promise<ShameBoardEntry[]> {
  try {
    const now = Date.now()
    if (cachedEntries && now - cacheTime < 30000) {
      return cachedEntries
    }
    const res = await fetchWithAbort(`${BACKEND_URL}/shame-board`, { cache: 'no-store' }, 'shame-board')
    if (!res.ok) {
      console.warn('GET /api/shame-board вернул', res.status)
      return []
    }
    const data = await res.json()
    const entries = Array.isArray(data) ? data : []
    cachedEntries = entries
    cacheTime = now
    return entries
  } catch (e) {
    if (!isAbortError(e)) {
      console.error('Ошибка загрузки доски позора:', e)
    }
    return []
  }
}

export async function checkShameBoard(): Promise<ShameCheckResult | null> {
  try {
    if (!tokens.adminKnockKey) await doAdminKnock()
    const res = await fetchWithAbort(`${BACKEND_URL}/shame-board/check`, {
      cache: 'no-store',
    }, 'shame-check')
    if (!res.ok) return null
    return await res.json()
  } catch (e) {
    if (!isAbortError(e)) {
      console.error('Ошибка проверки доски позора:', e)
    }
    return null
  }
}

export async function saveShameReason(discordId: string, reason: string): Promise<boolean> {
  try {
    if (!tokens.adminKnockKey) await doAdminKnock()
    const res = await fetchWithAbort(`${BACKEND_URL}/shame-board/save`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ discordId, reason })
    }, 'shame-save-reason')
    if (!res.ok) {
      const err = await parseJsonResponse(res)
      throw new Error((err.error as string) || 'Ошибка сохранения причины')
    }
    cachedEntries = null
    showToast('Причина сохранена', 'success')
    return true
  } catch (e) {
    if (!isAbortError(e)) {
      showToast((e as Error).message, 'error')
    }
    return false
  }
}

export async function deleteShameEntry(discordId: string): Promise<boolean> {
  try {
    if (!tokens.adminKnockKey) await doAdminKnock()
    const res = await fetchWithAbort(`${BACKEND_URL}/shame-board/delete`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ discordId })
    }, 'shame-delete')
    if (!res.ok) {
      const err = await parseJsonResponse(res)
      throw new Error((err.error as string) || 'Ошибка удаления')
    }
    cachedEntries = null
    showToast('Запись удалена', 'success')
    return true
  } catch (e) {
    if (!isAbortError(e)) {
      showToast((e as Error).message, 'error')
    }
    return false
  }
}

export async function syncShameBoard(): Promise<{ newCount: number; added: ShameBoardEntry[] } | null> {
  try {
    if (!tokens.adminKnockKey) await doAdminKnock()
    const res = await fetchWithAbort(`${BACKEND_URL}/shame-board/sync`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({})
    }, 'shame-sync')
    if (!res.ok) {
      const err = await parseJsonResponse(res)
      throw new Error((err.error as string) || 'Ошибка синхронизации')
    }
    const data = await res.json()
    cachedEntries = null
    return data
  } catch (e) {
    if (!isAbortError(e)) {
      showToast((e as Error).message, 'error')
    }
    return null
  }
}

export async function addManualEntry(username: string, discordId: string, reason: string): Promise<boolean> {
  try {
    if (!tokens.adminKnockKey) await doAdminKnock()
    const res = await fetchWithAbort(`${BACKEND_URL}/shame-board/add`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ username, discordId, reason })
    }, 'shame-add-manual')
    if (!res.ok) {
      const err = await parseJsonResponse(res)
      throw new Error((err.error as string) || 'Ошибка добавления')
    }
    cachedEntries = null
    showToast('Участник добавлен на Доску позора', 'success')
    return true
  } catch (e) {
    if (!isAbortError(e)) {
      showToast((e as Error).message, 'error')
    }
    return false
  }
}
