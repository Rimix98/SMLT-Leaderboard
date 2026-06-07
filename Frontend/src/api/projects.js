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
  if (!store.isHost) throw new Error('Только хост может сохранять проекты')
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

export function createDefaultParticipantConfig() {
  return {
    host: '',
    parts: [],
    endScreen: [],
    playtest: [],
    verifier: [],
    merger: [],
    merger2: [],
    showcaser: '',
    fxMode: false,
    soloGp: null,
  }
}

const ROLE_MAP = {
  HOST: 'host', GP: 'gp', DECO: 'deco', VERIFIER: 'verifier',
  PLAYTEST: 'playtest', MERGER: 'merger', MERGER2: 'merger2',
  TRANSITION: 'transition', SHOWCASER: 'showcaser', 'SOLO GP': 'soloGp',
  'END SCREEN': 'endScreen', ENDSCREEN: 'endScreen', FX: 'fx',
}

function assignOldRole(config, name, roles) {
  const upper = roles.map(r => r.toUpperCase())
  if (upper.includes('HOST')) { config.host = name; return }
  if (upper.includes('FX')) config.fxMode = true
  if (upper.includes('SOLO GP')) { config.soloGp = name; return }
  if (upper.includes('SHOWCASER')) { config.showcaser = name; return }
  if (upper.includes('TRANSITION') && config.parts.length > 0) {
    config.parts[config.parts.length - 1].transition = name; return
  }
  let hasKey = false
  for (const k of ['VERIFIER', 'PLAYTEST', 'MERGER', 'MERGER2', 'END SCREEN', 'ENDSCREEN']) {
    if (upper.includes(k)) {
      const key = ROLE_MAP[k]
      if (key && Array.isArray(config[key])) { config[key].push(name); hasKey = true; break }
    }
  }
  if (hasKey) return
  const gp = roles.some(r => r.toUpperCase() === 'GP')
  const deco = roles.some(r => r.toUpperCase() === 'DECO')
  if (gp || deco) {
    config.parts.push({ gp: gp ? [name] : [], deco: deco ? [name] : [], transition: '' })
    return
  }
  if (!config.host) config.host = name
  else config.parts.push({ gp: [name], deco: [], transition: '' })
}

function parseOldParticipantFormat(participants) {
  const config = createDefaultParticipantConfig()
  for (const line of participants) {
    if (typeof line !== 'string' || !line.trim()) continue
    const dashMatch = line.match(/^(.+?)\s*-\s+(.+)$/)
    if (dashMatch) {
      assignOldRole(config, dashMatch[1].trim(), dashMatch[2].split(/\s+/).filter(Boolean))
      continue
    }
    const parenMatch = line.match(/^(.+?)\s*\((.+?)\)$/)
    if (parenMatch) {
      assignOldRole(config, parenMatch[1].trim(), [parenMatch[2].trim()])
      continue
    }
    assignOldRole(config, line.trim(), [])
  }
  return config
}

export function parseParticipantConfig(project) {
  if (!project || !project.participants || project.participants.length === 0) {
    return createDefaultParticipantConfig()
  }
  if (project.participants.length > 1) {
    return parseOldParticipantFormat(project.participants)
  }
  try {
    const parsed = JSON.parse(project.participants[0])
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return { ...createDefaultParticipantConfig(), ...parsed }
    }
    if (Array.isArray(parsed)) {
      const config = createDefaultParticipantConfig()
      const parts = parsed.map(name => ({ gp: [String(name)], deco: [], transition: '' }))
      return { ...config, parts }
    }
  } catch {}
  return parseOldParticipantFormat(project.participants)
}

export function serializeParticipantConfig(config) {
  return [JSON.stringify(config)]
}

export function autoFillParticipantConfig() {
  const config = createDefaultParticipantConfig()
  const hostRole = (store.staffRoles || []).find(r => r.name === 'HOST')
  if (hostRole?.players?.[0]?.nickname) {
    config.host = hostRole.players[0].nickname
  }
  const gpRole = (store.staffRoles || []).find(r => r.name === 'GP')
  const decoRole = (store.staffRoles || []).find(r => r.name === 'DECO')
  if (gpRole?.players?.[0]?.nickname || decoRole?.players?.[0]?.nickname) {
    config.parts.push({
      gp: gpRole?.players?.[0]?.nickname ? [gpRole.players[0].nickname] : [],
      deco: decoRole?.players?.[0]?.nickname ? [decoRole.players[0].nickname] : [],
      transition: '',
    })
  }
  return config
}

export async function saveProject() {
  if (!store.isHost) { showToast('Только хост может сохранять проекты', 'error'); return }
  const idx = parseInt(document.getElementById('projectIndex').value)
  let projectId = document.getElementById('projectId').value.trim()
  if (idx === -1 && !projectId) {
    projectId = generateProjectId()
    document.getElementById('projectId').value = projectId
  }
  const MAX_NAME_LEN = 100
  function truncate(s) { return s && s.length > MAX_NAME_LEN ? s.slice(0, MAX_NAME_LEN) : s }

  const project = {
    name: truncate(document.getElementById('projectName').value.trim()),
    videoId: extractVideoId(document.getElementById('projectVideo').value.trim()),
    id: projectId,
    comment: truncate(document.getElementById('projectComment').value.trim()),
    status: document.getElementById('projectStatus').value,
    verifier: truncate(document.getElementById('projectVerifier').value.trim()),
    participants: idx === -1 ? [] : (store.projects[idx]?.participants || []),
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

  document.getElementById('projectModal').classList.add('active')
}

export function closeProjectModal() {
  const modal = document.getElementById('projectModal')
  if (modal) modal.classList.remove('active')
}

export function showAddProjectModal() {
  if (!store.isHost) { showToast('Только хост может добавлять проекты', 'error'); return }
  const modal = document.getElementById('projectModal')
  const form = document.getElementById('projectForm')
  if (modal && form) {
    form.reset()
    document.getElementById('projectIndex').value = '-1'
    document.getElementById('projectModalTitle').textContent = 'Добавить проект'
    modal.classList.add('active')
  }
}

export function moveProject(index, direction) {
  if (!store.isHost) { showToast('Только хост может перемещать проекты', 'error'); return }
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


