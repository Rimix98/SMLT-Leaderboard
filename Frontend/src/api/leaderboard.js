import { store } from '../store'
import { fetchWithAbort, parseJsonResponse, isAbortError, API_BASE, BACKEND_URL, doAdminKnock, tokens, showToast } from './utils'

const DEFAULT_PLAYER_NAMES = [
  "samoletik", "paradoxiz", "clokman", "itzslxnq", "H30n41k_GmD",
  "Filkoty", "DarBeast", "Florned", "Marzyiiik", "euphoriak8",
  "npoctou_gamer", "NopanicGD", "CandyCloud22", "Vakum", "Daggit",
  "Loran", "tapxyhh", "SerGio", "Fanim59", "prostoymofficial",
  "toxik blaze", "NatrixGMD", "toxatort", "SpaceRS", "yeahme",
  "Спини", "Linqwq", "RossceorpGD", "69liqu69"
]

export async function getPlayerNames() {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/players`, {}, 'players-list')
    if (!res.ok) return DEFAULT_PLAYER_NAMES
    const data = await res.json()
    if (Array.isArray(data) && data.length > 0) {
      if (typeof data[0] === 'object' && data[0].name) return data.map(p => p.name)
      return data
    }
    return DEFAULT_PLAYER_NAMES
  } catch {
    return DEFAULT_PLAYER_NAMES
  }
}

export async function savePlayerNames(names) {
  const formattedPlayers = names.map(n => ({ name: n }))
  if (!tokens.adminKnockKey) await doAdminKnock()
  const res = await fetchWithAbort(`${BACKEND_URL}/players/save`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify(formattedPlayers)
  }, 'players-save')
  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error || 'Ошибка сохранения игроков (возможно, сессия истекла)')
  }
}

function fetchPlayerData(name) {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 15000)
  return fetch(`${API_BASE}/leaderboard/user/list?search=${encodeURIComponent(name)}&limit=50`, { signal: controller.signal })
    .then(r => r.ok ? r.json() : null)
    .then(d => {
      if (d?.message !== 'success' || !d.data?.users?.length) return null
      const nl = name.toLowerCase().trim()
      const users = d.data.users
      let fp = users.find(p => p.username?.toLowerCase().trim() === nl)
      if (!fp && !isNaN(parseInt(name))) fp = users.find(p => p.id.toString() === name.trim())
      return fp || null
    })
    .catch(e => { console.error(`Ошибка для "${name}":`, e); return null })
    .finally(() => clearTimeout(timeout))
}

function fetchRecords(id) {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 15000)
  return fetch(`${API_BASE}/user/record/list?user_id=${id}&limit=50`, { signal: controller.signal })
    .then(r => r.ok ? r.json() : [])
    .then(d => d.message === 'success' && d.data?.records ? d.data.records : [])
    .catch(() => [])
    .finally(() => clearTimeout(timeout))
}

function mapLeaderboardEntry(p) {
  let userData = null
  if (p.data && p.data.data && Array.isArray(p.data.data.users) && p.data.data.users.length > 0) {
    userData = p.data.data.users[0]
  } else if (p.data && Array.isArray(p.data.users) && p.data.users.length > 0) {
    userData = p.data.users[0]
  }

  let pRecs = []
  if (p.records && p.records.data && Array.isArray(p.records.data.records)) {
    pRecs = p.records.data.records
  } else if (p.records && Array.isArray(p.records.records)) {
    pRecs = p.records.records
  }

  let hardest = null
  const completedRecs = pRecs.filter(r => r.status === 'accepted' && r.level && (r.percent ?? r.progress ?? 100) >= 100)
  if (completedRecs.length > 0) {
    hardest = completedRecs.reduce((m, r) => (!m || r.level.placement < m.level.placement) ? r : m)
  }

  return {
    id: userData?.id || p.id,
    name: p.name,
    rank: userData?.placement || 0,
    score: parseFloat(userData?.points) || 0,
    nationality: userData?.country || null,
    records: pRecs,
    hardest
  }
}

let _loadingLeaderboard = false

export async function loadAllPlayers() {
  if (_loadingLeaderboard) return
  _loadingLeaderboard = true

  try {
    let playersToMap = []
    const res = await fetchWithAbort('/api/leaderboard', {}, 'leaderboard')
    if (res.ok) {
      const responseData = await parseJsonResponse(res)
      if (Array.isArray(responseData)) playersToMap = responseData
    }

    if (playersToMap.length === 0) {
      await loadPlayersFromClientAPI()
      return
    }

    const loaded = playersToMap.map(mapLeaderboardEntry).filter(p => p.id)
    if (loaded.length === 0) {
      await loadPlayersFromClientAPI()
      return
    }

    store.players = loaded.sort((a, b) => (a.rank || 999999) - (b.rank || 999999))
    store.allPlayers = [...store.players]
    renderHardestLevels()
  } catch (e) {
    if (isAbortError(e)) return
    try { await loadPlayersFromClientAPI() } catch (err) { if (isAbortError(err)) return }
  } finally {
    _loadingLeaderboard = false
  }
}

async function loadPlayersFromClientAPI() {
  const names = await getPlayerNames()

  const promises = names.map(async (name) => {
    try {
      const fp = await fetchPlayerData(name)
      if (!fp) return null
      const recs = await fetchRecords(fp.id)
      let hardest = null
      const completedRecs = recs.filter(r => r.status === 'accepted' && r.level && (r.percent ?? r.progress ?? 100) >= 100)
      if (completedRecs.length > 0) {
        hardest = completedRecs.reduce((m, r) => (!m || r.level.placement < m.level.placement) ? r : m)
      }
      return {
        id: fp.id,
        name: fp.username || name,
        rank: fp.placement || 0,
        score: parseFloat(fp.points) || 0,
        nationality: fp.country || null,
        records: recs,
        hardest
      }
    } catch (e) { console.error(`Ошибка загрузки игрока ${name}:`, e); return null }
  })

  const results = await Promise.all(promises)
  const loaded = results.filter(p => p !== null)
  if (loaded.length === 0) return

  store.players = loaded.sort((a, b) => (a.rank || 999999) - (b.rank || 999999))
  store.allPlayers = [...store.players]
  renderHardestLevels()
}

export function filterPlayers(query) {
  if (!query) {
    store.players = [...store.allPlayers]
  } else {
    const q = query.toLowerCase().trim()
    store.players = store.allPlayers.filter(p => p.name.toLowerCase().includes(q))
  }
}

function renderHardestLevels() {
  const levelMap = new Map()

  store.players.forEach(player => {
    if (player.records) {
      player.records.forEach(record => {
        if (record.status === 'accepted' && record.level && (record.percent ?? record.progress ?? 100) >= 100) {
          const levelId = record.level.id
          if (!levelMap.has(levelId)) {
            levelMap.set(levelId, {
              id: levelId,
              name: record.level.name,
              placement: record.level.placement,
              victors: []
            })
          }
          const levelData = levelMap.get(levelId)
          if (!levelData.victors.find(v => v.id === player.id)) {
            levelData.victors.push({ id: player.id, name: player.name, nationality: player.nationality })
          }
        }
      })
    }
  })

  const sortedLevels = Array.from(levelMap.values())
    .filter(level => level.placement !== undefined && level.placement !== null)
    .sort((a, b) => a.placement - b.placement)

  if (sortedLevels.length === 0) {
    store.levels.all = null
    store.levels.levelData = null
    return
  }

  store.levels.all = sortedLevels
  store.levels.expanded = false
  store.levels.filter = ''
  store.levels.levelData = new Map()
  for (const [k, v] of levelMap) {
    store.levels.levelData.set(String(k), v)
  }
}

export function getFilteredLevels() {
  if (!store.levels.all) return []
  const q = store.levels.filter?.toLowerCase().trim()
  let list = q ? store.levels.all.filter(l => l.name.toLowerCase().includes(q)) : store.levels.all
  if (!store.levels.expanded) list = list.slice(0, 39)
  return list
}

export function expandLevels() {
  store.levels.expanded = !store.levels.expanded
}

export function filterLevels(query) {
  store.levels.filter = query
}

export async function addPlayer(name) {
  if (!store.isHost) { showToast('Только хост может добавлять игроков', 'error'); return }
  if (!name) return
  if (name.length < 2 || name.length > 32) { showToast('Ник должен быть от 2 до 32 символов', 'error'); return }

  let playerNames = await getPlayerNames()
  if (playerNames.includes(name)) { showToast('Такой игрок уже есть', 'error'); return }

  playerNames.push(name)
  try {
    await savePlayerNames(playerNames)
    await loadAllPlayers()
    showToast('Игрок успешно добавлен', 'success')
  } catch (e) {
    if (isAbortError(e)) return
    showToast(e.message, 'error')
    if (e.message.includes('сессия истекла') || e.message.includes('401')) {
      const { logoutHost } = await import('./auth')
      logoutHost()
    }
  }
}

export async function removePlayer(name) {
  if (!store.isHost) { showToast('Только хост может удалять игроков', 'error'); return }
  if (!confirm(`Удалить игрока "${name}"?`)) return
  await doAdminKnock()

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/players/delete`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ name })
    }, 'players-delete')
    if (res.ok) { await loadAllPlayers(); showToast(`Игрок "${name}" удалён`, 'success'); return }
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error || 'Ошибка удаления игрока')
  } catch (e) {
    if (isAbortError(e)) return
    showToast(e.message, 'error')
    if (e.message.includes('сессия') || e.message.includes('401') || e.message.includes('доступ')) {
      const { logoutHost } = await import('./auth')
      logoutHost()
    }
  }
}
