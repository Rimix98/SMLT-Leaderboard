import { store } from '../store'
import { fetchWithAbort, parseJsonResponse, isAbortError, BACKEND_URL, showToast, tokens, doAdminKnock } from './utils'

export const TIER_CONFIG = {
  priority: { label: 'Приоритет', color: '#00ffff' },
  base: { label: 'Основа', color: '#540b6d' },
  reserve: { label: 'Резерв', color: '#6d0b0d' },
  na: { label: 'N/A', color: '#888888' },
}

const TIER_CYCLE = ['na', 'priority', 'base', 'reserve']

export function getNextTier(current) {
  const idx = TIER_CYCLE.indexOf(current)
  if (idx === -1 || idx >= TIER_CYCLE.length - 1) return TIER_CYCLE[0]
  return TIER_CYCLE[idx + 1]
}

export function getPlayerTier(nickname) {
  const entry = store.staffTiers.find(t => t.nickname === nickname)
  return entry ? entry.tier : 'na'
}

export function getTierConfig(nickname) {
  return TIER_CONFIG[getPlayerTier(nickname)] || TIER_CONFIG.na
}

function sortRolePlayersByTiers(role) {
  if (!role || !role.players) return
  const tierOrder = { priority: 0, base: 1, reserve: 2, na: 3 }
  role.players = [...role.players].sort((a, b) => {
    const ta = tierOrder[getPlayerTier(a.nickname)] ?? 3
    const tb = tierOrder[getPlayerTier(b.nickname)] ?? 3
    if (ta !== tb) return ta - tb
    return a.nickname.localeCompare(b.nickname)
  })
}

function sortRolesByTierDistribution(roleA, roleB) {
  const getCounts = (role) => {
    const counts = { priority: 0, base: 0, reserve: 0, na: 0 }
    ;(role.players || []).forEach(p => {
      const tier = getPlayerTier(p.nickname)
      if (counts[tier] !== undefined) counts[tier]++
    })
    return counts
  }

  const countsA = getCounts(roleA)
  const countsB = getCounts(roleB)

  for (const tier of ['priority', 'base', 'reserve']) {
    if (countsA[tier] !== countsB[tier]) {
      return countsB[tier] - countsA[tier]
    }
  }

  return (roleA.name || '').localeCompare(roleB.name || '')
}

export function sortAllRolesByTiers() {
  if (!store.staffRoles) return
  store.staffRoles = [...store.staffRoles].sort(sortRolesByTierDistribution)
  for (const role of store.staffRoles) {
    sortRolePlayersByTiers(role)
  }
}

export async function loadStaffRoles() {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff`, {}, 'staff-list')
    if (!res.ok) { console.warn('GET /api/staff вернул', res.status); store.staffRoles = [] }
    else { const data = await res.json(); store.staffRoles = Array.isArray(data) ? data : [] }
    sortAllRolesByTiers()
  } catch (e) {
    if (!isAbortError(e)) { console.error('Ошибка загрузки staff ролей:', e); store.staffRoles = [] }
  }
}

export async function loadStaffTiers() {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/tiers`, {}, 'staff-tiers')
    if (res.ok) { const data = await res.json(); store.staffTiers = Array.isArray(data.gp) ? data.gp : [] }
    else { store.staffTiers = [] }
    sortAllRolesByTiers()
  } catch (e) {
    if (!isAbortError(e)) { console.error('Ошибка загрузки тиров:', e); store.staffTiers = [] }
  }
}

export async function saveStaffRoles() {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/save`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(store.staffRoles)
    }, 'staff-save')
    if (!res.ok) { const err = await res.json().catch(() => ({})); throw new Error(err.error || 'Ошибка сохранения ролей') }
  } catch (e) {
    if (!isAbortError(e)) { console.error('Ошибка сохранения staff ролей:', e); showToast(e.message, 'error') }
  }
}

export async function addPlayerToRoleApi(roleIndex, nickname, discord) {
  try {
    if (!tokens.adminKnockKey) await doAdminKnock()
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/add`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex, nickname, discord })
    }, 'add-player')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка добавления игрока') }
    await Promise.all([loadStaffRoles(), loadStaffTiers()])
    return true
  } catch (e) {
    if (!isAbortError(e)) { showToast(e.message, 'error') }
    return false
  }
}

export async function removeStaffPlayer(roleIndex, playerIndex) {
  const role = store.staffRoles[roleIndex]
  if (!role) return
  const player = role.players[playerIndex]
  if (!player) return
  if (!confirm(`Удалить игрока «${player.nickname}» из роли «${role.name}»?`)) return

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/remove`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex, nickname: player.nickname })
    }, 'remove-player')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка удаления игрока') }
    await Promise.all([loadStaffRoles(), loadStaffTiers()])
    showToast(`Игрок «${player.nickname}» удалён из роли`, 'success')
  } catch (e) {
    if (!isAbortError(e)) { showToast(e.message, 'error') }
  }
}

export function moveRole(index, direction) {
  const target = direction === 'down' ? index + 1 : index - 1
  if (target < 0 || target >= store.staffRoles.length) return
  const prev = [...store.staffRoles]
  ;[store.staffRoles[index], store.staffRoles[target]] = [store.staffRoles[target], store.staffRoles[index]]
  fetchWithAbort(`${BACKEND_URL}/staff/reorder`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ roleIndex: index, direction })
  }, 'staff-reorder').catch(() => { store.staffRoles = prev })
}

export async function deleteRole(index) {
  const role = store.staffRoles[index]
  if (!role) return
  if (!confirm(`Удалить роль «${role.name}»?`)) return

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex: index })
    }, 'delete-role')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка удаления роли') }
    await loadStaffRoles()
    showToast('Роль удалена', 'success')
  } catch (e) {
    if (!isAbortError(e)) { showToast(e.message, 'error') }
  }
}

export async function createRoleApi(name, color) {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ name, color })
    }, 'create-role')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка создания роли') }
    await loadStaffRoles()
    showToast(`Роль «${name}» создана`, 'success')
    return true
  } catch (e) {
    if (!isAbortError(e)) { showToast(e.message, 'error') }
    return false
  }
}

export async function updateRoleApi(roleIndex, name, color, tiersEnabled) {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex, name, color, tiersEnabled })
    }, 'update-role')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка обновления роли') }
    await loadStaffRoles()
    showToast(`Роль «${name}» обновлена`, 'success')
    return true
  } catch (e) {
    if (!isAbortError(e)) { showToast(e.message, 'error') }
    return false
  }
}

export async function setPlayerTier(nickname, tier) {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/tier`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ category: 'gp', nickname, tier })
    }, 'set-tier')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка установки тира') }
    await Promise.all([loadStaffRoles(), loadStaffTiers()])
    showToast(`Тир для «${nickname}» установлен: ${TIER_CONFIG[tier]?.label || tier}`, 'success')
    return true
  } catch (e) {
    if (!isAbortError(e)) { showToast(e.message, 'error') }
    return false
  }
}

export async function toggleRoleTiers(roleIndex) {
  const role = store.staffRoles[roleIndex]
  if (!role) return
  const newName = role.name
  const newEnabled = role.tiersEnabled === false ? true : false
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex, name: role.name, color: role.color, tiersEnabled: newEnabled })
    }, 'role-toggle-tiers')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка') }
    await loadStaffRoles()
    showToast(`Тиры для роли «${newName}» ${newEnabled ? 'включены' : 'выключены'}`, 'success')
    return true
  } catch (e) {
    showToast(e.message, 'error')
    return false
  }
}

export async function removePlayerFromRoleApi(roleIndex, nickname) {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/remove`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex, nickname })
    }, 'remove-player-modal')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка удаления игрока') }
    await Promise.all([loadStaffRoles(), loadStaffTiers()])
    showToast(`Игрок «${nickname}» удалён из роли`, 'success')
    return true
  } catch (e) {
    if (!isAbortError(e)) { showToast(e.message, 'error') }
    return false
  }
}

export { sortRolePlayersByTiers }
