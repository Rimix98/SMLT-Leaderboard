import { store } from '../store'
import { fetchWithAbort, parseJsonResponse, isAbortError, BACKEND_URL, doAdminKnock, tokens, refreshCsrfToken, showToast } from './utils'

const DEFAULT_PROJECTS = []

export async function getProjects() {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/projects`, {}, 'projects-list')
    if (!res.ok) return []
    const data = await res.json()
    return Array.isArray(data) ? data : []
  } catch { return [] }
}

export async function saveProjects(data) {
  for (let attempt = 0; attempt < 2; attempt++) {
    if (!tokens.adminKnockKey) await doAdminKnock()
    const res = await fetchWithAbort(`${BACKEND_URL}/projects/save`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify(data)
    }, 'projects-save')
    if (res.ok) return
    if (res.status === 404 && attempt === 0) { tokens.adminKnockKey = ''; continue }
    if (res.status === 403 && attempt === 0) { tokens.csrfToken = ''; await refreshCsrfToken(); continue }
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error || 'Ошибка сохранения (возможно, сессия истекла)')
  }
}

function loadProjectOrder() {
  try {
    const raw = localStorage.getItem('smlt-project-order')
    if (!raw) return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.filter(id => typeof id === 'string') : []
  } catch { return [] }
}

function saveProjectOrder(order) {
  localStorage.setItem('smlt-project-order', JSON.stringify(order))
}

export function syncProjectOrder() {
  const order = store.projects.map(p => p.id).filter(Boolean)
  saveProjectOrder(order)
}

function sortProjectsByOrder() {
  const order = loadProjectOrder()
  if (order.length === 0) return
  const map = {}; const multi = new Set()
  store.projects.forEach((p, i) => { if (p.id in map) multi.add(p.id); map[p.id] = i })
  const validOrder = order.filter(id => id in map && !multi.has(id))
  const unsorted = store.projects.filter(p => !validOrder.includes(p.id))
  store.projects = [...validOrder.map(id => store.projects[map[id]]), ...unsorted]
}

export async function loadProjects() {
  store.projects = await getProjects()
  sortProjectsByOrder()
}

export function getStatusClass(status) {
  const classes = {
    'готов': 'status-ready',
    'в процессе верифа': 'status-verifying',
    'в процессе постройки': 'status-building',
    'планируется': 'status-planned',
    'заморожен': 'status-frozen',
    'мёртв': 'status-dead'
  }
  return classes[status?.toLowerCase()] || 'status-planned'
}

export function extractVideoId(url) {
  if (!url) return ''
  const patterns = [
    /(?:youtube\.com\/(?:watch\?v=|embed\/|shorts\/)|youtu\.be\/)([a-zA-Z0-9_-]{11})/,
    /^([a-zA-Z0-9_-]{11})$/
  ]
  for (const pattern of patterns) {
    const match = url.match(pattern)
    if (match) return match[1]
  }
  return ''
}

export function toYoutubeId11(raw) {
  const id = extractVideoId(String(raw || ''))
  return id && /^[a-zA-Z0-9_-]{11}$/.test(id) ? id : null
}

export function generateProjectId() {
  return 'proj_' + Date.now().toString(36) + '_' + Math.random().toString(36).slice(2, 8)
}

export async function saveProject() {
  const idx = parseInt(document.getElementById('projectIndex').value)
  let projectId = document.getElementById('projectId').value.trim()
  if (idx === -1 && !projectId) {
    projectId = generateProjectId()
    document.getElementById('projectId').value = projectId
  }
  const project = {
    name: document.getElementById('projectName').value.trim(),
    videoId: extractVideoId(document.getElementById('projectVideo').value.trim()),
    id: projectId,
    comment: document.getElementById('projectComment').value.trim(),
    status: document.getElementById('projectStatus').value,
    verifier: document.getElementById('projectVerifier').value.trim(),
    participants: Array.isArray(store.pendingProjectParticipants) ? store.pendingProjectParticipants.filter(Boolean) : []
  }

  if (projectId !== '-') {
    const isDuplicate = store.projects.some((p, i) => i !== idx && p.id === projectId)
    if (isDuplicate) { showToast('Проект с таким ID уже существует!', 'error'); return }
  }

  const oldProject = idx === -1 ? null : { ...store.projects[idx] }
  if (idx === -1) { store.projects.push(project) } else { store.projects[idx] = project }

  try {
    syncProjectOrder()
    await saveProjects(store.projects)
    await loadProjects()
    showToast(idx === -1 ? 'Проект добавлен!' : 'Проект обновлён!', 'success')
    closeProjectModal()
  } catch (e) {
    if (isAbortError(e)) return
    if (idx === -1) { store.projects.pop() } else { store.projects[idx] = oldProject }
    showToast(e.message, 'error')
  }
}

export async function deleteProject(idx) {
  if (!store.isHost) { showToast('Только хост может удалять проекты', 'error'); return }
  if (!confirm('Удалить этот проект?')) return

  const removed = store.projects.splice(idx, 1)
  syncProjectOrder()
  try {
    await saveProjects(store.projects)
    await loadProjects()
    showToast('Проект удалён', 'success')
  } catch (e) {
    if (isAbortError(e)) return
    store.projects.splice(idx, 0, removed[0])
    syncProjectOrder()
    showToast(e.message, 'error')
  }
}

export function editProject(idx) {
  if (!store.isHost) { showToast('Только хост может редактировать проекты', 'error'); return }
  const project = store.projects[idx]
  if (!project) return

  document.getElementById('projectIndex').value = idx
  document.getElementById('projectModalTitle').textContent = 'Редактировать проект'
  document.getElementById('projectName').value = project.name || ''
  document.getElementById('projectVideo').value = project.videoId || ''
  document.getElementById('projectId').value = project.id || ''
  document.getElementById('projectComment').value = project.comment || ''
  document.getElementById('projectStatus').value = project.status || 'планируется'
  document.getElementById('projectVerifier').value = project.verifier || ''

  store.pendingProjectParticipants = [...(project.participants || [])]
  updateParticipantsPreview()

  document.getElementById('projectModal').classList.add('active')
  setTimeout(initParticipantBuilder, 50)
}

export function closeProjectModal() {
  const modal = document.getElementById('projectModal')
  if (modal) modal.classList.remove('active')
  resetParticipantBuilder()
}

export function showAddProjectModal() {
  if (!store.isHost) { showToast('Только хост может добавлять проекты', 'error'); return }
  const modal = document.getElementById('projectModal')
  const form = document.getElementById('projectForm')
  if (modal && form) {
    form.reset()
    document.getElementById('projectIndex').value = '-1'
    document.getElementById('projectModalTitle').textContent = 'Добавить проект'
    resetParticipantBuilder()
    modal.classList.add('active')
    setTimeout(initParticipantBuilder, 50)
  }
}

export function moveProject(index, direction) {
  const target = direction === 'down' ? index + 1 : index - 1
  if (target < 0 || target >= store.projects.length) return
  ;[store.projects[index], store.projects[target]] = [store.projects[target], store.projects[index]]
  syncProjectOrder()
  saveProjects(store.projects).catch(e => {
    ;[store.projects[index], store.projects[target]] = [store.projects[target], store.projects[index]]
    syncProjectOrder()
    showToast(e.message, 'error')
  })
}

// Participant builder
export function resetParticipantBuilder() {
  store.pendingProjectParticipants = []
  store._selectedParticipant = ''
  const preview = document.getElementById('participantsPreview')
  if (preview) preview.innerHTML = ''
  const searchInput = document.getElementById('participantSearchInput')
  if (searchInput) searchInput.value = ''
  const results = document.getElementById('participantSearchResults')
  if (results) results.innerHTML = ''
  document.querySelectorAll('#participantRoleTags .role-tag-btn').forEach(b => {
    b.classList.remove('active')
    b.style.background = ''
    b.style.color = b.dataset.color || 'var(--color-text-secondary)'
    b.style.borderColor = b.dataset.color || 'var(--color-border)'
  })
}

export async function initParticipantBuilder() {
  if (!store.staffRoles || store.staffRoles.length === 0) {
    try {
      const res = await fetchWithAbort(`${BACKEND_URL}/staff`, {}, 'staff-list-part')
      if (res.ok) {
        const data = await res.json()
        store.staffRoles = Array.isArray(data) ? data : []
      }
    } catch {}
  }

  const tagsContainer = document.getElementById('participantRoleTags')
  if (tagsContainer) {
    tagsContainer.innerHTML = ''
    ;(store.staffRoles || []).forEach(role => {
      const btn = document.createElement('button')
      btn.type = 'button'
      btn.className = 'role-tag-btn'
      btn.dataset.action = 'toggle-role-tag'
      btn.dataset.role = role.name
      if (role.color) { btn.dataset.color = role.color; btn.style.borderColor = role.color; btn.style.color = role.color }
      btn.textContent = role.name
      tagsContainer.appendChild(btn)
    })
  }

  updateParticipantsPreview()

  const searchInput = document.getElementById('participantSearchInput')
  const resultsContainer = document.getElementById('participantSearchResults')
  if (searchInput && resultsContainer) {
    searchInput.value = ''
    resultsContainer.innerHTML = ''
    searchInput.oninput = null
    searchInput.oninput = function() {
      const q = this.value.toLowerCase().trim()
      resultsContainer.innerHTML = ''
      if (!q) { resultsContainer.style.display = 'none'; return }
      const matches = []
      ;(store.staffRoles || []).forEach(role => {
        ;(role.players || []).forEach(player => {
          if (player.nickname.toLowerCase().includes(q) && !matches.some(m => m.nickname === player.nickname)) {
            matches.push({ nickname: player.nickname, role: role.name, color: role.color })
          }
        })
      })
      matches.sort((a, b) => a.nickname.localeCompare(b.nickname))
      if (matches.length === 0) { resultsContainer.style.display = 'none'; return }
      resultsContainer.style.display = 'block'
      matches.forEach(m => {
        const item = document.createElement('div')
        item.className = 'participant-search-result-item'
        if (m.color) item.style.borderLeftColor = m.color
        const psrName = document.createElement('span')
        psrName.className = 'psr-name'
        psrName.textContent = m.nickname
        const psrRole = document.createElement('span')
        psrRole.className = 'psr-role'
        psrRole.textContent = m.role
        item.appendChild(psrName)
        item.appendChild(document.createTextNode(' '))
        item.appendChild(psrRole)
        item.addEventListener('click', () => {
          document.querySelectorAll('.participant-search-result-item').forEach(el => el.classList.remove('selected'))
          item.classList.add('selected')
          store._selectedParticipant = m.nickname
          resultsContainer.style.display = 'none'
          searchInput.value = m.nickname
        })
        resultsContainer.appendChild(item)
      })
    }
    searchInput.addEventListener('blur', () => setTimeout(() => { resultsContainer.style.display = 'none' }, 150))
    searchInput.addEventListener('focus', () => { if (searchInput.value.trim()) searchInput.oninput() })
  }
  store._selectedParticipant = ''
}

export function addProjectParticipant() {
  const name = store._selectedParticipant || document.getElementById('participantSearchInput')?.value?.trim()
  if (!name) { showToast('Введите или выберите игрока', 'error'); return }
  const activeRoles = []
  document.querySelectorAll('#participantRoleTags .role-tag-btn.active').forEach(b => activeRoles.push(b.dataset.role))
  let entry = name
  if (activeRoles.length > 0) entry = name + ' - ' + activeRoles.join(' ')
  store.pendingProjectParticipants.push(entry)
  store._selectedParticipant = ''
  const searchInput = document.getElementById('participantSearchInput')
  if (searchInput) searchInput.value = ''
  const results = document.getElementById('participantSearchResults')
  if (results) results.innerHTML = ''
  document.querySelectorAll('#participantRoleTags .role-tag-btn').forEach(b => {
    b.classList.remove('active')
    b.style.background = ''
    b.style.color = b.dataset.color || 'var(--color-text-secondary)'
    b.style.borderColor = b.dataset.color || 'var(--color-border)'
  })
  updateParticipantsPreview()
}

export function removeProjectParticipant(index) {
  if (index >= 0 && index < store.pendingProjectParticipants.length) {
    store.pendingProjectParticipants.splice(index, 1)
    updateParticipantsPreview()
  }
}

export function updateParticipantsPreview() {
  const preview = document.getElementById('participantsPreview')
  if (!preview) return
  preview.innerHTML = ''
  if (!store.pendingProjectParticipants || store.pendingProjectParticipants.length === 0) {
    preview.innerHTML = '<span style="color:var(--color-text-muted);font-size:var(--font-size-xs)">Участники не добавлены</span>'
    return
  }
  store.pendingProjectParticipants.forEach((entry, i) => {
    const tag = document.createElement('span')
    tag.className = 'participant-tag participant-preview-tag'
    const newMatch = entry.match(/^(.+?)\s*-\s+(.+)$/)
    if (newMatch) {
      const name = newMatch[1].trim()
      const roles = newMatch[2].split(/\s+/).filter(Boolean)
      tag.appendChild(document.createTextNode(`${name} - `))
      roles.forEach((role, ri) => {
        if (ri) tag.appendChild(document.createTextNode(' '))
        const roleSpan = document.createElement('span')
        roleSpan.className = 'role'
        const roleObj = (store.staffRoles || []).find(r => r.name === role)
        if (roleObj?.color) roleSpan.style.color = roleObj.color
        roleSpan.textContent = role
        tag.appendChild(roleSpan)
      })
    } else {
      tag.textContent = entry
    }
    const removeBtn = document.createElement('button')
    removeBtn.className = 'staff-player-remove-tag'
    removeBtn.dataset.action = 'remove-project-participant'
    removeBtn.dataset.index = String(i)
    removeBtn.title = 'Удалить'
    removeBtn.textContent = '✕'
    tag.appendChild(removeBtn)
    preview.appendChild(tag)
  })
}
