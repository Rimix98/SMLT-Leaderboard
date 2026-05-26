import { store } from '../store'
import { fetchWithAbort, parseJsonResponse, isAbortError, BACKEND_URL, showToast } from './utils'

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

export async function loadStaffRoles() {
  const loadingState = document.getElementById('staffLoadingState')
  if (loadingState) loadingState.style.display = 'flex'

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff`, {}, 'staff-list')
    if (!res.ok) { console.warn('GET /api/staff вернул', res.status); store.staffRoles = [] }
    else { const data = await res.json(); store.staffRoles = Array.isArray(data) ? data : [] }
  } catch (e) {
    if (!isAbortError(e)) { console.error('Ошибка загрузки staff ролей:', e); store.staffRoles = [] }
  } finally {
    if (loadingState) loadingState.style.display = 'none'
  }
}

export async function loadStaffTiers() {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/tiers`, {}, 'staff-tiers')
    if (res.ok) { const data = await res.json(); store.staffTiers = Array.isArray(data.gp) ? data.gp : [] }
    else { store.staffTiers = [] }
  } catch (e) {
    if (!isAbortError(e)) { console.error('Ошибка загрузки тиров:', e); store.staffTiers = [] }
  }
}

export async function saveStaffRoles() {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff`, {
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

export function getRoleColor(roleName) {
  if (!store.staffRoles) return null
  const role = store.staffRoles.find(r => r.name === roleName)
  return role ? role.color : null
}

function getRoleColorCSS(roleText) {
  return getRoleColor(roleText) || null
}

export async function addPlayerToRole() {
  const roleIndexInput = document.getElementById('addPlayerRoleIndex')
  const nicknameInput = document.getElementById('playerNickname')
  const discordInput = document.getElementById('playerDiscord')

  const roleIndex = parseInt(roleIndexInput?.value || '-1')
  if (roleIndex < 0 || roleIndex >= store.staffRoles.length) { showToast('Ошибка: роль не найдена', 'error'); return }

  const nickname = nicknameInput?.value?.trim()
  if (!nickname) { showToast('Введите ник игрока', 'error'); return }

  const discord = discordInput?.value?.trim() || ''
  const role = store.staffRoles[roleIndex]

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/add`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex, nickname, discord })
    }, 'add-player')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка добавления игрока') }

    await Promise.all([loadStaffRoles(), loadStaffTiers()])
    closeAddStaffPlayerModal()
    showToast(`Игрок «${nickname}» добавлен в роль «${role.name}»`, 'success')
  } catch (e) {
    if (!isAbortError(e)) { console.error('Ошибка добавления игрока:', e); showToast(e.message, 'error') }
  }
}

export function closeAddStaffPlayerModal() {
  const modal = document.getElementById('addPlayerModal')
  if (modal) modal.classList.remove('active')
}

export function showAddStaffPlayerModal(roleIndex) {
  const modal = document.getElementById('addPlayerModal')
  const title = document.getElementById('addPlayerModalTitle')
  const roleIndexInput = document.getElementById('addPlayerRoleIndex')
  if (modal && title && roleIndexInput) {
    const role = store.staffRoles[roleIndex]
    if (!role) return
    title.textContent = `➕ Добавить игрока в «${role.name}»`
    roleIndexInput.value = roleIndex
    const n = document.getElementById('playerNickname'); if (n) n.value = ''
    const d = document.getElementById('playerDiscord'); if (d) d.value = ''
    modal.classList.add('active')
    setTimeout(() => { const f = document.getElementById('playerNickname'); if (f) f.focus() }, 100)
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
    if (!isAbortError(e)) { console.error('Ошибка удаления игрока:', e); showToast(e.message, 'error') }
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
    if (!isAbortError(e)) { console.error('Ошибка удаления роли:', e); showToast(e.message, 'error') }
  }
}

export function showAddRoleModal() {
  const modal = document.getElementById('addRoleModal')
  if (modal) {
    document.getElementById('editRoleIndex').value = '-1'
    document.getElementById('roleName').value = ''
    document.getElementById('roleColor').value = store.selectedRoleColor || '#3b82f6'
    const hexInput = document.getElementById('roleColorHex')
    if (hexInput) hexInput.value = (store.selectedRoleColor || '#3b82f6').replace('#', '')
    document.getElementById('addRoleModalTitle').textContent = '🆕 Новая роль'
    document.getElementById('createRoleBtn').textContent = 'Создать'
    const playerSection = document.getElementById('rolePlayerSection')
    if (playerSection) playerSection.style.display = 'none'
    modal.classList.add('active')
    setTimeout(() => { const f = document.getElementById('roleName'); if (f) f.focus() }, 100)
  }
}

export function closeAddRoleModal() {
  const modal = document.getElementById('addRoleModal')
  if (modal) modal.classList.remove('active')
}

export async function createRole() {
  const editIndexInput = document.getElementById('editRoleIndex')
  const editIndex = parseInt(editIndexInput?.value || '-1')
  if (editIndex >= 0) { await updateRole(editIndex); return }

  const name = document.getElementById('roleName').value.trim()
  if (!name) { showToast('Введите название роли', 'error'); return }
  const color = document.getElementById('roleColor').value || '#3b82f6'

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ name, color })
    }, 'create-role')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка создания роли') }
    await loadStaffRoles()
    closeAddRoleModal()
    document.getElementById('roleName').value = ''
    showToast(`Роль «${name}» создана`, 'success')
  } catch (e) {
    if (!isAbortError(e)) { console.error('Ошибка создания роли:', e); showToast(e.message, 'error') }
  }
}

async function updateRole(roleIndex) {
  const name = document.getElementById('roleName').value.trim()
  if (!name) { showToast('Введите название роли', 'error'); return }
  const color = document.getElementById('roleColor').value || '#3b82f6'

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex, name, color })
    }, 'update-role')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка обновления роли') }
    await loadStaffRoles()
    closeAddRoleModal()
    showToast(`Роль «${name}» обновлена`, 'success')
  } catch (e) {
    if (!isAbortError(e)) { console.error('Ошибка обновления роли:', e); showToast(e.message, 'error') }
  }
}

export function showEditRoleModal(roleIndex) {
  const role = store.staffRoles[roleIndex]
  if (!role) return
  const modal = document.getElementById('addRoleModal')
  if (modal) {
    document.getElementById('editRoleIndex').value = roleIndex
    document.getElementById('roleName').value = role.name
    const color = role.color || '#3b82f6'
    document.getElementById('roleColor').value = color
    const hexInput = document.getElementById('roleColorHex')
    if (hexInput) hexInput.value = color.replace('#', '')
    store.selectedRoleColor = color
    document.getElementById('addRoleModalTitle').textContent = '✏️ Редактировать роль'
    document.getElementById('createRoleBtn').textContent = 'Сохранить'
    const playerSection = document.getElementById('rolePlayerSection')
    if (playerSection) playerSection.style.display = 'block'
    document.getElementById('roleAddPlayerNickname').value = ''
    document.getElementById('roleAddPlayerDiscord').value = ''
    const searchInput = document.getElementById('rolePlayerSearch')
    if (searchInput) searchInput.value = ''
    const addBtn = document.getElementById('roleAddPlayerBtn')
    if (addBtn) { addBtn.textContent = '➕ Добавить'; addBtn.dataset.action = 'role-add-player' }
    const toggleBtn = document.getElementById('roleToggleTiersBtn')
    if (toggleBtn) toggleBtn.textContent = role.tiersEnabled !== false ? '🎯 Тир: вкл' : '🎯 Тир: выкл'
    renderRoleModalPlayerList(roleIndex)
    modal.classList.add('active')
    setTimeout(() => { const f = document.getElementById('roleName'); if (f) f.focus() }, 100)
  }
}

export function renderRoleModalPlayerList(roleIndex) {
  const container = document.getElementById('rolePlayerList')
  if (!container) return
  container.innerHTML = ''

  const role = store.staffRoles[roleIndex]
  if (!role) return

  const searchInput = document.getElementById('rolePlayerSearch')
  const query = searchInput ? searchInput.value.toLowerCase().trim() : ''

  const tiersEnabled = role.tiersEnabled !== false
  const players = role.players || []
  if (players.length === 0) {
    container.innerHTML = '<span style="color:var(--color-text-muted);font-size:var(--font-size-xs)">Нет игроков</span>'
    return
  }

  let count = 0
  for (let pIdx = 0; pIdx < players.length; pIdx++) {
    const p = players[pIdx]
    if (query && !p.nickname.toLowerCase().includes(query)) continue
    count++

    let actionsHTML = ''
    if (tiersEnabled) {
      Object.entries(TIER_CONFIG).forEach(([key, cfg]) => {
        const isActive = getPlayerTier(p.nickname) === key
        actionsHTML += `<span class="role-tier-square" style="background:${cfg.color};opacity:${isActive ? 1 : 0.25};outline:${isActive ? '2px solid var(--color-text-primary)' : 'none'};outlineOffset:1px" data-action="role-modal-set-tier-direct" data-nickname="${p.nickname}" data-tier="${key}" title="${cfg.label}"></span>`
      })
    }
    actionsHTML += `<button class="player-edit-btn" data-action="role-modal-edit-player" data-role-index="${roleIndex}" data-player-index="${pIdx}" title="Редактировать">✏️</button>`
    if (pIdx > 0) actionsHTML += `<button class="player-edit-btn" data-action="role-modal-move-player" data-role-index="${roleIndex}" data-player-index="${pIdx}" data-direction="up" title="Вверх">↑</button>`
    if (pIdx < players.length - 1) actionsHTML += `<button class="player-edit-btn" data-action="role-modal-move-player" data-role-index="${roleIndex}" data-player-index="${pIdx}" data-direction="down" title="Вниз">↓</button>`
    actionsHTML += `<button class="player-remove-btn" data-action="role-modal-remove-player" data-role-index="${roleIndex}" data-nickname="${p.nickname}" title="Удалить">✕</button>`

    const item = document.createElement('div')
    item.className = 'edit-player-list-item'
    item.innerHTML = `<div class="player-info"><span class="player-nickname">${p.nickname}</span>${p.discord ? `<span class="player-role-name">${p.discord}</span>` : ''}</div>${actionsHTML}`
    container.appendChild(item)
  }

  if (count === 0) {
    container.innerHTML = `<span style="color:var(--color-text-muted);font-size:var(--font-size-xs)">${query ? 'Ничего не найдено' : 'Нет игроков'}</span>`
  }
}

export async function addPlayerFromRoleModal() {
  const roleIndex = parseInt(document.getElementById('editRoleIndex')?.value || '-1')
  if (roleIndex < 0 || roleIndex >= store.staffRoles.length) { showToast('Ошибка: роль не найдена', 'error'); return }

  const nicknameInput = document.getElementById('roleAddPlayerNickname')
  const discordInput = document.getElementById('roleAddPlayerDiscord')
  const nickname = nicknameInput?.value?.trim()
  if (!nickname) { showToast('Введите ник игрока', 'error'); return }
  const discord = discordInput?.value?.trim() || ''

  document.getElementById('editRolePlayerIdx').value = '-1'
  document.getElementById('roleAddPlayerNickname').value = ''
  document.getElementById('roleAddPlayerDiscord').value = ''
  const btn = document.getElementById('roleAddPlayerBtn')
  if (btn) { btn.textContent = '➕ Добавить'; btn.dataset.action = 'role-add-player' }

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/add`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex, nickname, discord })
    }, 'role-add-player')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка добавления игрока') }
    await Promise.all([loadStaffRoles(), loadStaffTiers()])
    nicknameInput.value = ''; discordInput.value = ''
    renderRoleModalPlayerList(roleIndex)
    showToast(`Игрок «${nickname}» добавлен в роль «${store.staffRoles[roleIndex]?.name}»`, 'success')
  } catch (e) {
    if (!isAbortError(e)) { console.error('Ошибка добавления игрока:', e); showToast(e.message, 'error') }
  }
}

export async function roleModalSortByTiers(roleIndex) {
  if (roleIndex == null) roleIndex = parseInt(document.getElementById('editRoleIndex')?.value || '-1')
  const role = store.staffRoles[roleIndex]
  if (!role || !role.players) return
  const tierOrder = { priority: 0, base: 1, reserve: 2, na: 3 }
  role.players.sort((a, b) => {
    const ta = tierOrder[getPlayerTier(a.nickname)] ?? 3
    const tb = tierOrder[getPlayerTier(b.nickname)] ?? 3
    if (ta !== tb) return ta - tb
    return a.nickname.localeCompare(b.nickname)
  })
  renderRoleModalPlayerList(roleIndex)
  showToast('Участники отсортированы по тирам', 'success')
}

export async function roleModalToggleTiers(roleIndex) {
  if (roleIndex == null) roleIndex = parseInt(document.getElementById('editRoleIndex')?.value || '-1')
  const role = store.staffRoles[roleIndex]
  if (!role) return
  role.tiersEnabled = role.tiersEnabled === false ? true : false
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/role`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex, name: role.name, color: role.color, tiersEnabled: role.tiersEnabled })
    }, 'role-toggle-tiers')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка') }
    await loadStaffRoles()
    const btn = document.getElementById('roleToggleTiersBtn')
    if (btn) btn.textContent = role.tiersEnabled ? '🎯 Тир: вкл' : '🎯 Тир: выкл'
    renderRoleModalPlayerList(roleIndex)
    showToast(`Тиры для роли «${role.name}» ${role.tiersEnabled ? 'включены' : 'выключены'}`, 'success')
  } catch (e) {
    role.tiersEnabled = !role.tiersEnabled
    showToast(e.message, 'error')
  }
}

export function openEditPanel() {
  if (!store.isHost) return
  document.getElementById('editPanelOverlay')?.classList.add('active')
  document.getElementById('editPanel')?.classList.add('open')
  document.body.style.overflow = 'hidden'
  populateEditRoleSelect()
  document.getElementById('editPlayerKey').value = ''
  const searchInput = document.getElementById('editPlayerSearch')
  if (searchInput) searchInput.value = ''
  renderEditPlayerList()
  document.getElementById('editPlayerNickname').value = ''
  document.getElementById('editPlayerDiscord').value = ''
  const btn = document.getElementById('editPanelSubmitBtn')
  if (btn) { btn.textContent = '➕ Добавить игрока'; btn.dataset.action = 'edit-add-player' }
  setTimeout(() => { const f = document.getElementById('editPlayerNickname'); if (f) f.focus() }, 100)
}

export function closeEditPanel() {
  document.getElementById('editPanelOverlay')?.classList.remove('active')
  document.getElementById('editPanel')?.classList.remove('open')
  document.body.style.overflow = ''
}

function populateEditRoleSelect() {
  const select = document.getElementById('editPlayerRole')
  if (!select) return
  select.innerHTML = ''
  const placeholder = document.createElement('option')
  placeholder.value = ''; placeholder.disabled = true; placeholder.selected = true
  placeholder.textContent = 'Выберите роль...'
  select.appendChild(placeholder)
  store.staffRoles.forEach((role, idx) => {
    const opt = document.createElement('option')
    opt.value = String(idx); opt.textContent = role.name
    select.appendChild(opt)
  })
}

function renderEditPlayerList() {
  const container = document.getElementById('editPlayerList')
  if (!container || !store.isHost) return
  container.innerHTML = ''
  const searchInput = document.getElementById('editPlayerSearch')
  const query = searchInput ? searchInput.value.toLowerCase().trim() : ''

  let totalPlayers = 0
  for (const role of store.staffRoles) {
    const rolePlayers = role.players || []
    for (const p of rolePlayers) {
      if (query && !p.nickname.toLowerCase().includes(query)) continue
      totalPlayers++
      const tier = getPlayerTier(p.nickname)
      const roleIndex = store.staffRoles.indexOf(role)
      const playerIndex = role.players.indexOf(p)

      let tierHTML = ''
      Object.entries(TIER_CONFIG).forEach(([key, cfg]) => {
        const isActive = getPlayerTier(p.nickname) === key
        tierHTML += `<span class="role-tier-square" style="background:${cfg.color};opacity:${isActive ? 1 : 0.25};outline:${isActive ? '2px solid var(--color-text-primary)' : 'none'};outlineOffset:1px" data-action="role-modal-set-tier-direct" data-nickname="${p.nickname}" data-tier="${key}" title="${cfg.label}"></span>`
      })

      const item = document.createElement('div')
      item.className = 'edit-player-list-item'
      item.innerHTML = `
        <div class="player-info">
          <span class="player-nickname">${p.nickname}</span>
          <span class="player-role-name">${role.name}</span>
        </div>
        ${tierHTML}
        <button class="player-edit-btn" data-action="edit-player-from-list" data-role-index="${roleIndex}" data-player-index="${playerIndex}" title="Редактировать">✏️</button>
        <button class="player-remove-btn" data-action="edit-remove-player" data-role-index="${roleIndex}" data-nickname="${p.nickname}" title="Удалить игрока">✕</button>
      `
      container.appendChild(item)
    }
  }
  if (totalPlayers === 0) {
    container.innerHTML = `<span style="color:var(--color-text-muted);font-size:var(--font-size-xs)">${query ? 'Ничего не найдено' : 'Нет игроков'}</span>`
  }
}

export async function editAddPlayer() {
  const nicknameInput = document.getElementById('editPlayerNickname')
  const discordInput = document.getElementById('editPlayerDiscord')
  const roleSelect = document.getElementById('editPlayerRole')
  const key = document.getElementById('editPlayerKey').value

  const nickname = nicknameInput.value.trim()
  if (!nickname) { showToast('Введите ник игрока', 'error'); return }

  const roleIndex = parseInt(roleSelect.value)
  if (isNaN(roleIndex) || roleIndex < 0 || roleIndex >= store.staffRoles.length) { showToast('Выберите роль', 'error'); return }
  const discord = discordInput.value.trim() || ''

  if (key) {
    const [oldRoleIdx, playerIdx] = key.split(':').map(Number)
    const oldRole = store.staffRoles[oldRoleIdx]
    const role = store.staffRoles[roleIndex]
    if (oldRoleIdx !== roleIndex) {
      if (oldRole && oldRole.players) oldRole.players.splice(playerIdx, 1)
      if (!role.players) role.players = []
      role.players.push({ nickname, discord })
    } else {
      if (role.players && role.players[playerIdx]) { role.players[playerIdx].nickname = nickname; role.players[playerIdx].discord = discord }
    }
    try {
      await saveStaffRoles()
      await loadStaffTiers()
      document.getElementById('editPlayerKey').value = ''
      nicknameInput.value = ''; discordInput.value = ''
      const btn = document.getElementById('editPanelSubmitBtn')
      if (btn) { btn.textContent = '➕ Добавить игрока'; btn.dataset.action = 'edit-add-player' }
      renderEditPlayerList()
      showToast(`Игрок «${nickname}» обновлён`, 'success')
    } catch (e) { showToast(e.message, 'error') }
    return
  }

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff/add`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ roleIndex, nickname, discord })
    }, 'edit-add-player')
    if (!res.ok) { const err = await parseJsonResponse(res); throw new Error(err.error || 'Ошибка добавления игрока') }
    await Promise.all([loadStaffRoles(), loadStaffTiers()])
    nicknameInput.value = ''; discordInput.value = ''
    renderEditPlayerList()
    showToast(`Игрок «${nickname}» добавлен в роль «${store.staffRoles[roleIndex].name}»`, 'success')
  } catch (e) {
    if (!isAbortError(e)) { console.error('Ошибка добавления игрока:', e); showToast(e.message, 'error') }
  }
}
