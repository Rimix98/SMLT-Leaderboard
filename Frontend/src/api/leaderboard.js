import { store } from '../store'
import { fetchWithAbort, parseJsonResponse, isAbortError, API_BASE, BACKEND_URL, doAdminKnock, tokens, showToast, createFlagElement } from './utils'

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

function hasLeaderboardData(resData) {
  return Array.isArray(resData)
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

export function renderHardestLevels() {
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

export function showLevelVictors(levelId) {
  const levelData = store.levels.levelData?.get(String(levelId))
  if (!levelData) return

  const modal = document.getElementById('levelModal')
  const title = document.getElementById('levelTitle')
  const body = document.getElementById('levelBody')
  if (!modal || !title || !body) return

  title.textContent = `🏆 ${levelData.name} #${levelData.placement}`
  while (body.firstChild) body.firstChild.remove()

  if (levelData.victors.length === 0) {
    const p = document.createElement('p')
    p.style.cssText = 'color: var(--color-text-muted);'
    p.textContent = 'Нет викторов'
    body.appendChild(p)
    modal.classList.add('active')
    return
  }

  const list = document.createElement('div')
  list.className = 'level-victors-list'
  levelData.victors.forEach((victor, idx) => {
    const flagEl = createFlagElement(victor.nationality)
    const item = document.createElement('div')
    item.style.cssText = 'display:flex;justify-content:space-between;padding:var(--spacing-sm);border-bottom:1px solid var(--color-border)'
    const span = document.createElement('span')
    const strong = document.createElement('strong')
    strong.textContent = `#${idx + 1} `
    span.appendChild(strong)
    span.appendChild(flagEl)
    span.appendChild(document.createTextNode(` ${victor.name}`))
    item.appendChild(span)
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

  const titleEl = document.getElementById('profileTitle')
  while (titleEl.firstChild) titleEl.firstChild.remove()
  const flagEl = createFlagElement(p.nationality)
  titleEl.appendChild(flagEl)
  titleEl.appendChild(document.createTextNode(` ${p.name}`))

  const score = p.score ? p.score.toFixed(2) : '—'
  const rank = p.rank || '—'
  const body = document.getElementById('profileBody')
  while (body.firstChild) body.firstChild.remove()

  function makeStat(value, label) {
    const div = document.createElement('div')
    div.className = 'profile-stat'
    const valDiv = document.createElement('div')
    valDiv.className = 'profile-stat-value'
    valDiv.textContent = String(value)
    const lblDiv = document.createElement('div')
    lblDiv.className = 'profile-stat-label'
    lblDiv.textContent = label
    div.appendChild(valDiv)
    div.appendChild(lblDiv)
    return div
  }

  const statsDiv = document.createElement('div')
  statsDiv.className = 'profile-stats'
  statsDiv.appendChild(makeStat(score, 'Очки'))
  statsDiv.appendChild(makeStat(`#${rank}`, 'Глобальный топ'))
  statsDiv.appendChild(makeStat(rec.length, 'Уровней'))
  body.appendChild(statsDiv)

  function makeInfoRow(label, valueNode) {
    const row = document.createElement('div')
    row.className = 'profile-info-row'
    const lbl = document.createElement('span')
    lbl.className = 'profile-info-label'
    lbl.textContent = label
    row.appendChild(lbl)
    const val = document.createElement('span')
    val.className = 'profile-info-value'
    if (valueNode instanceof Node) {
      val.appendChild(valueNode)
    } else {
      val.textContent = String(valueNode)
    }
    row.appendChild(val)
    return row
  }

  if (p.hardest) {
    const hardestLabel = p.hardest.level?.name != null ? String(p.hardest.level.name) : String(p.hardest)
    body.appendChild(makeInfoRow('Hardest:', hardestLabel))
  }

  const countryVal = document.createElement('span')
  countryVal.className = 'profile-info-value'
  const countryFlag = createFlagElement(p.nationality)
  countryVal.appendChild(countryFlag)
  countryVal.appendChild(document.createTextNode(` ${p.nationality || 'Не указана'}`))
  body.appendChild(makeInfoRow('Страна:', countryVal))

  const recordsSection = document.createElement('div')
  recordsSection.className = 'profile-records-section'
  const h4 = document.createElement('h4')
  h4.textContent = `Пройденные уровни (${rec.length})`
  recordsSection.appendChild(h4)
  const recordsList = document.createElement('div')
  recordsList.className = 'profile-records-list'

  if (rec.length > 0) {
    rec.forEach(r => {
      const levelName = r.level?.name || 'Unknown'
      const placement = r.level?.placement ?? '?'
      const progress = r.percent ?? r.progress ?? 100
      const item = document.createElement('div')
      item.className = 'record-item'
      const demonSpan = document.createElement('span')
      demonSpan.className = 'record-demon'
      demonSpan.textContent = levelName
      const placeSpan = document.createElement('span')
      placeSpan.className = 'record-placement'
      placeSpan.textContent = `#${placement}`
      demonSpan.appendChild(placeSpan)
      item.appendChild(demonSpan)
      const progSpan = document.createElement('span')
      progSpan.className = `record-progress${progress >= 100 ? ' progress-100' : ''}`
      progSpan.textContent = `${progress}%`
      item.appendChild(progSpan)
      recordsList.appendChild(item)
    })
  } else {
    const noRec = document.createElement('div')
    noRec.className = 'no-records'
    noRec.textContent = 'Нет записей'
    recordsList.appendChild(noRec)
  }
  recordsSection.appendChild(recordsList)
  body.appendChild(recordsSection)

  const linkDiv = document.createElement('div')
  linkDiv.className = 'profile-link'
  const a = document.createElement('a')
  a.href = `https://demonlist.org/profile/${encodeURIComponent(String(p.id))}/`
  a.target = '_blank'
  a.rel = 'noopener noreferrer'
  a.textContent = '🔗 Показать аккаунт в Global Demonlist →'
  linkDiv.appendChild(a)
  body.appendChild(linkDiv)

  document.getElementById('profileModal').classList.add('active')
}

export function closeProfileModal(e) {
  if (!e || e.target === e.currentTarget) {
    document.getElementById('profileModal').classList.remove('active')
  }
}

export function showCountryTop(raw) {
  import('./utils').then(({ resolveCountry, CODE_TO_NAME }) => {
    const modal = document.getElementById('countryModal')
    const title = document.getElementById('countryTitle')
    const body = document.getElementById('countryBody')
    if (!modal || !title || !body) return

    let countryPlayers
    let flagEl
    let countryName

    if (!raw) {
      countryPlayers = store.allPlayers.filter(p => !p.nationality)
        .sort((a, b) => (a.rank || 999999) - (b.rank || 999999))
      const span = document.createElement('span')
      span.textContent = '🌍'
      flagEl = span
      countryName = 'Unknown'
    } else {
      const country = resolveCountry(raw)
      if (!country) { showToast('Страна не найдена', 'error'); return }

      countryPlayers = store.allPlayers.filter(p => resolveCountry(p.nationality) === country)
        .sort((a, b) => (a.rank || 999999) - (b.rank || 999999))
      flagEl = createFlagElement(country)
      countryName = CODE_TO_NAME[country] || country
    }

    while (title.firstChild) title.firstChild.remove()
    title.appendChild(flagEl)
    title.appendChild(document.createTextNode(` Топ игроков: ${countryName}`))

    while (body.firstChild) body.firstChild.remove()

    if (countryPlayers.length === 0) {
      const p = document.createElement('p')
      p.style.cssText = 'color: var(--color-text-muted);'
      p.textContent = 'Нет данных'
      body.appendChild(p)
    } else {
      countryPlayers.forEach((p, idx) => {
        const score = p.score ? p.score.toFixed(2) : '—'
        const rank = p.rank || '—'
        const row = document.createElement('div')
        row.style.cssText = 'display:flex;justify-content:space-between;padding:var(--spacing-sm);border-bottom:1px solid var(--color-border)'
        const leftSpan = document.createElement('span')
        const strong = document.createElement('strong')
        strong.textContent = `#${idx + 1} `
        leftSpan.appendChild(strong)
        leftSpan.appendChild(document.createTextNode(p.name))
        row.appendChild(leftSpan)
        const rightSpan = document.createElement('span')
        rightSpan.style.cssText = 'color:var(--color-text-muted)'
        rightSpan.textContent = `${score} pts · #${rank}`
        row.appendChild(rightSpan)
        body.appendChild(row)
      })
    }

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
