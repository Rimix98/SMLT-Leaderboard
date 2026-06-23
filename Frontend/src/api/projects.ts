import { store } from '../store'
import { fetchWithAbort, isAbortError, BACKEND_URL, doAdminKnock, tokens, refreshCsrfToken, showToast } from './utils'
import type { Project, ParticipantConfig, StaffRole } from '../types'

export async function getProjects(): Promise<Project[]> {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/projects`, {}, 'projects-list')
    if (!res.ok) return []
    const data = await res.json()
    return Array.isArray(data) ? data : []
  } catch { return [] }
}

export async function saveProjects(data: Project[]): Promise<void> {
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
    throw new Error((err.error as string) || 'Ошибка сохранения (возможно, сессия истекла)')
  }
}

function loadProjectOrder(): string[] {
  try {
    const raw = localStorage.getItem('smlt-project-order')
    if (!raw) return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.filter((id: unknown) => typeof id === 'string') : []
  } catch { return [] }
}

function saveProjectOrder(order: string[]): void {
  localStorage.setItem('smlt-project-order', JSON.stringify(order))
}

export function syncProjectOrder(): void {
  const order = store.projects.map(p => p.id).filter(Boolean)
  saveProjectOrder(order)
}

function sortProjectsByOrder(): void {
  const order = loadProjectOrder()
  if (order.length === 0) return
  const map: Record<string, number> = {}; const multi = new Set<string>()
  store.projects.forEach((p, i) => { if (p.id in map) multi.add(p.id); map[p.id] = i })
  const validOrder = order.filter(id => id in map && !multi.has(id))
  const unsorted = store.projects.filter(p => !validOrder.includes(p.id))
  store.projects = [...validOrder.map(id => store.projects[map[id]]), ...unsorted]
}

export async function loadProjects(): Promise<void> {
  store.projects = await getProjects()
  sortProjectsByOrder()
}

export function getStatusClass(status: string): string {
  const classes: Record<string, string> = {
    'готов': 'status-ready',
    'в процессе верифа': 'status-verifying',
    'в процессе постройки': 'status-building',
    'планируется': 'status-planned',
    'заморожен': 'status-frozen',
    'мёртв': 'status-dead'
  }
  return classes[status?.toLowerCase()] || 'status-planned'
}

export function extractVideoId(url: string): string {
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

export function toYoutubeId11(raw: string): string | null {
  const id = extractVideoId(String(raw || ''))
  return id && /^[a-zA-Z0-9_-]{11}$/.test(id) ? id : null
}

export function generateProjectId(): string {
  return 'proj_' + Date.now().toString(36) + '_' + Math.random().toString(36).slice(2, 8)
}

export function createDefaultParticipantConfig(): ParticipantConfig {
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

const ROLE_MAP: Record<string, string> = {
  HOST: 'host', GP: 'gp', DECO: 'deco', VERIFIER: 'verifier',
  PLAYTEST: 'playtest', MERGER: 'merger', MERGER2: 'merger2',
  TRANSITION: 'transition', SHOWCASER: 'showcaser', 'SOLO GP': 'soloGp',
  'END SCREEN': 'endScreen', ENDSCREEN: 'endScreen', FX: 'fx',
}

function assignOldRole(config: ParticipantConfig, name: string, roles: string[]): void {
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
      if (key && Array.isArray(config[key as keyof ParticipantConfig])) {
        ;(config[key as keyof ParticipantConfig] as string[]).push(name)
        hasKey = true
        break
      }
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

function parseOldParticipantFormat(participants: string[]): ParticipantConfig {
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

export function parseParticipantConfig(project: { participants?: string[] }): ParticipantConfig {
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
      const parts = parsed.map((name: string) => ({ gp: [String(name)], deco: [], transition: '' }))
      return { ...config, parts }
    }
  } catch { /* ignore */ }
  return parseOldParticipantFormat(project.participants)
}

export function serializeParticipantConfig(config: ParticipantConfig): string[] {
  return [JSON.stringify(config)]
}

export function autoFillParticipantConfig(): ParticipantConfig {
  const config = createDefaultParticipantConfig()
  const hostRole = (store.staffRoles || []).find((r: StaffRole) => r.name === 'HOST')
  if (hostRole?.players?.[0]?.nickname) {
    config.host = hostRole.players[0].nickname
  }
  const gpRole = (store.staffRoles || []).find((r: StaffRole) => r.name === 'GP')
  const decoRole = (store.staffRoles || []).find((r: StaffRole) => r.name === 'DECO')
  if (gpRole?.players?.[0]?.nickname || decoRole?.players?.[0]?.nickname) {
    config.parts.push({
      gp: gpRole?.players?.[0]?.nickname ? [gpRole.players[0].nickname] : [],
      deco: decoRole?.players?.[0]?.nickname ? [decoRole.players[0].nickname] : [],
      transition: '',
    })
  }
  return config
}

export interface ProjectFormData {
  name: string
  videoId: string
  id: string
  comment: string
  status: string
  verifier: string
}

const MAX_NAME_LEN = 100
function truncate(s: string): string { return s && s.length > MAX_NAME_LEN ? s.slice(0, MAX_NAME_LEN) : s }

export function saveProjectFromForm(formData: ProjectFormData, editingIdx: number): void {
  if (!store.isHost) { showToast('Только хост может сохранять проекты', 'error'); return }
  const idx = editingIdx
  let projectId = formData.id.trim()
  if (idx === -1 && !projectId) {
    projectId = generateProjectId()
  }

  const project: Project = {
    name: truncate(formData.name.trim()),
    videoId: extractVideoId(formData.videoId.trim()),
    id: projectId,
    comment: truncate(formData.comment.trim()),
    status: formData.status,
    verifier: truncate(formData.verifier.trim()),
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
    saveProjects(store.projects).then(() => {
      loadProjects()
      showToast(idx === -1 ? 'Проект добавлен!' : 'Проект обновлён!', 'success')
    }).catch((e: Error) => {
      if (idx === -1) { store.projects.pop() } else if (oldProject) { store.projects[idx] = oldProject }
      showToast(e.message, 'error')
    })
  } catch (e) {
    if (idx === -1) { store.projects.pop() } else if (oldProject) { store.projects[idx] = oldProject }
    showToast((e as Error).message, 'error')
  }
}

export async function deleteProject(idx: number): Promise<void> {
  if (!store.isHost) { showToast('Только хост может удалять проекты', 'error'); return }
  if (!confirm('Удалить этот проект?')) return

  const removed = store.projects.splice(idx, 1)
  syncProjectOrder()
  try {
    await saveProjects(store.projects)
    showToast('Проект удалён', 'success')
  } catch (e) {
    if (isAbortError(e)) return
    store.projects.splice(idx, 0, removed[0])
    syncProjectOrder()
    showToast((e as Error).message, 'error')
  }
}

export function moveProject(index: number, direction: 'up' | 'down'): void {
  if (!store.isHost) { showToast('Только хост может перемещать проекты', 'error'); return }
  const target = direction === 'down' ? index + 1 : index - 1
  if (target < 0 || target >= store.projects.length) return
  ;[store.projects[index], store.projects[target]] = [store.projects[target], store.projects[index]]
  syncProjectOrder()
  saveProjects(store.projects).catch((e: Error) => {
    ;[store.projects[index], store.projects[target]] = [store.projects[target], store.projects[index]]
    syncProjectOrder()
    showToast(e.message, 'error')
  })
}
