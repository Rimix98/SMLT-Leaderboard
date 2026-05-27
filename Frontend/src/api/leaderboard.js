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
  return fetch(`${API_BASE}/leaderboard/user/list?search=${encodeURIComponent(name)}&limit=50`)
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
}

function fetchRecords(id) {
  return fetch(`${API_BASE}/user/record/list?user_id=${id}&limit=50`)
    .then(r => r.ok ? r.json() : [])
    .then(d => d.message === 'success' && d.data?.records ? d.data.records : [])
    .catch(() => [])
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
  const acceptedRecs = pRecs.filter(r => r.status === 'accepted' && r.level)
  if (acceptedRecs.length > 0) {
    hardest = acceptedRecs.reduce((m, r) => (!m || r.level.placement < m.level.placement) ? r : m)
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

function hasLeaderboardData(resData) {
  return Array.isArray(resData)
}

let _loadingLeaderboard = false

export async function loadAllPlayers() {
  if (_loadingLeaderboard) return
  _loadingLeaderboard = true

  try {
    const table = document.getElementById('leaderboardTable')
    if (!table) { _loadingLeaderboard = false; return }

    let playersToMap = []
    const res = await fetchWithAbort('/api/leaderboard', {}, 'leaderboard')
    if (res.ok) {
      const responseData = await parseJsonResponse(res)
      if (hasLeaderboardData(responseData)) playersToMap = responseData
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
  const table = document.getElementById('leaderboardTable')
  const names = await getPlayerNames()

  const promises = names.map(async (name) => {
    try {
      const fp = await fetchPlayerData(name)
      if (!fp) return null
      const recs = await fetchRecords(fp.id)
      let hardest = null
      const acceptedRecs = recs.filter(r => r.status === 'accepted' && r.level)
      if (acceptedRecs.length > 0) {
        hardest = acceptedRecs.reduce((m, r) => (!m || r.level.placement < m.level.placement) ? r : m)
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

export function renderHardestLevels() {
  const levelMap = new Map()

  store.players.forEach(player => {
    if (player.records) {
      player.records.forEach(record => {
        if (record.status === 'accepted' && record.level) {
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

export function showLevelVictors(levelId) {
  const levelData = store.levels.levelData?.get(String(levelId))
  if (!levelData) return

  const modal = document.getElementById('levelModal')
  const title = document.getElementById('levelTitle')
  const body = document.getElementById('levelBody')
  if (!modal || !title || !body) return

  title.textContent = `🏆 ${levelData.name} #${levelData.placement}`
  body.innerHTML = ''

  if (levelData.victors.length === 0) {
    body.innerHTML = '<p style="color: var(--color-text-muted);">Нет викторов</p>'
    modal.classList.add('active')
    return
  }

  const list = document.createElement('div')
  list.className = 'level-victors-list'
  levelData.victors.forEach((victor, idx) => {
    const flagHTML = `<img src="https://flagcdn.com/w20/${(victor.nationality || '').toLowerCase()}.png" alt="${victor.nationality || ''}" width="20" style="vertical-align:middle;border-radius:2px;margin-right:4px">`
    const item = document.createElement('div')
    item.style.cssText = 'display:flex;justify-content:space-between;padding:var(--spacing-sm);border-bottom:1px solid var(--color-border)'
    item.innerHTML = `<span><strong>#${idx + 1}</strong> ${flagHTML} ${victor.name}</span>`
    list.appendChild(item)
  })
  body.appendChild(list)
  modal.classList.add('active')
}

export function closeLevelModal() {
  const modal = document.getElementById('levelModal')
  if (modal) modal.classList.remove('active')
}

export function showProfile(idx) {
  const p = store.players[idx]
  if (!p) return

  const rec = p.records ? p.records.filter(r => r.status === 'accepted' && r.level) : []
  const flagHTML = `<img src="https://flagcdn.com/w20/${(p.nationality || '').toLowerCase()}.png" alt="${p.nationality || ''}" width="20" style="vertical-align:middle;border-radius:2px;margin-right:4px">`

  const titleEl = document.getElementById('profileTitle')
  titleEl.innerHTML = `${flagHTML} ${p.name}`

  const score = p.score ? p.score.toFixed(2) : '—'
  const rank = p.rank || '—'
  const body = document.getElementById('profileBody')
  body.innerHTML = ''

  const statsDiv = document.createElement('div')
  statsDiv.className = 'profile-stats'
  statsDiv.innerHTML = `
    <div class="profile-stat"><div class="profile-stat-value">${score}</div><div class="profile-stat-label">Очки</div></div>
    <div class="profile-stat"><div class="profile-stat-value">#${rank}</div><div class="profile-stat-label">Глобальный топ</div></div>
    <div class="profile-stat"><div class="profile-stat-value">${rec.length}</div><div class="profile-stat-label">Уровней</div></div>
  `
  body.appendChild(statsDiv)

  if (p.hardest) {
    const hardestLabel = p.hardest.level?.name != null ? String(p.hardest.level.name) : String(p.hardest)
    const row = document.createElement('div')
    row.className = 'profile-info-row'
    row.innerHTML = `<span class="profile-info-label">Hardest:</span><span class="profile-info-value">${hardestLabel}</span>`
    body.appendChild(row)
  }

  const countryRow = document.createElement('div')
  countryRow.className = 'profile-info-row'
  countryRow.innerHTML = `<span class="profile-info-label">Страна:</span><span class="profile-info-value">${flagHTML} ${p.nationality || 'Не указана'}</span>`
  body.appendChild(countryRow)

  const recordsSection = document.createElement('div')
  recordsSection.className = 'profile-records-section'
  recordsSection.innerHTML = `<h4>Пройденные уровни (${rec.length})</h4><div class="profile-records-list"></div>`
  const recordsList = recordsSection.querySelector('.profile-records-list')

  if (rec.length > 0) {
    rec.forEach(r => {
      const levelName = r.level?.name || 'Unknown'
      const placement = r.level?.placement ?? '?'
      const progress = r.percent ?? r.progress ?? 100
      const item = document.createElement('div')
      item.className = 'record-item'
      item.innerHTML = `<span class="record-demon">${levelName}<span class="record-placement">#${placement}</span></span><span class="record-progress${progress >= 100 ? ' progress-100' : ''}">${progress}%</span>`
      recordsList.appendChild(item)
    })
  } else {
    recordsList.innerHTML = '<div class="no-records">Нет записей</div>'
  }
  body.appendChild(recordsSection)

  const linkDiv = document.createElement('div')
  linkDiv.className = 'profile-link'
  linkDiv.innerHTML = `<a href="https://demonlist.org/profile/${encodeURIComponent(String(p.id))}/" target="_blank" rel="noopener noreferrer">🔗 Показать аккаунт в Global Demonlist →</a>`
  body.appendChild(linkDiv)

  document.getElementById('profileModal').classList.add('active')
}

export function closeProfileModal(e) {
  if (!e || e.target === e.currentTarget) {
    document.getElementById('profileModal').classList.remove('active')
  }
}

export function showCountryTop(raw) {
  import('./utils').then(({ resolveCountry, CODE_TO_NAME, getFlagHTML }) => {
    const country = resolveCountry(raw)
    if (!country) { showToast('Страна не найдена', 'error'); return }

    const countryPlayers = store.allPlayers.filter(p => resolveCountry(p.nationality) === country)
      .sort((a, b) => (a.rank || 999999) - (b.rank || 999999))

    const modal = document.getElementById('countryModal')
    const title = document.getElementById('countryTitle')
    const body = document.getElementById('countryBody')
    if (!modal || !title || !body) return

    const flagHTML = getFlagHTML(country)
    const countryName = CODE_TO_NAME[country] || country
    title.innerHTML = `${flagHTML} Топ игроков: ${countryName}`

    body.innerHTML = countryPlayers.length === 0
      ? '<p style="color: var(--color-text-muted);">Нет данных</p>'
      : countryPlayers.map((p, idx) => {
          const score = p.score ? p.score.toFixed(2) : '—'
          const rank = p.rank || '—'
          return `<div style="display:flex;justify-content:space-between;padding:var(--spacing-sm);border-bottom:1px solid var(--color-border)">
            <span><strong>#${idx + 1}</strong> ${p.name}</span>
            <span style="color:var(--color-text-muted)">${score} pts · #${rank}</span>
          </div>`
        }).join('')

    modal.classList.add('active')
  })
}

export function closeCountryModal() {
  const modal = document.getElementById('countryModal')
  if (modal) modal.classList.remove('active')
}

export function showInfoModal() {
  const modal = document.getElementById('infoModal')
  if (modal) modal.classList.add('active')
}

export function closeInfoModal(e) {
  if (!e || e.target === e.currentTarget) {
    const modal = document.getElementById('infoModal')
    if (modal) modal.classList.remove('active')
  }
}

export async function addPlayer() {
  if (!store.isHost) { showToast('Только хост может добавлять игроков', 'error'); return }

  const nameInput = document.getElementById('newPlayerName')
  const name = nameInput.value.trim()
  if (!name) return
  if (name.length < 2 || name.length > 32) { showToast('Ник должен быть от 2 до 32 символов', 'error'); return }

  let playerNames = await getPlayerNames()
  if (playerNames.includes(name)) { showToast('Такой игрок уже есть', 'error'); return }

  playerNames.push(name)
  try {
    await savePlayerNames(playerNames)
    closeAddPlayerModal()
    nameInput.value = ''
    await loadAllPlayers()
    showToast('Игрок успешно добавлен', 'success')
  } catch (e) {
    if (isAbortError(e)) return
    showToast(e.message, 'error')
    if (e.message.includes('сессия истекла') || e.message.includes('401')) logoutHost()
  }
}

export function closeAddPlayerModal() {
  const modal = document.getElementById('addPlayerModal')
  if (modal) modal.classList.remove('active')
}

export function showAddPlayerModal() {
  if (!store.isHost) { showToast('Только хост может добавлять игроков', 'error'); return }
  const modal = document.getElementById('addPlayerModal')
  if (modal) {
    document.getElementById('newPlayerName').value = ''
    modal.classList.add('active')
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
    if (e.message.includes('сессия') || e.message.includes('401') || e.message.includes('доступ')) logoutHost()
  }
}

// Fix circular import for logoutHost
async function logoutHost() {
  const { logoutHost: doLogout } = await import('./auth')
  doLogout()
}

export { logoutHost }
