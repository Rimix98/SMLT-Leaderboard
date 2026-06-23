<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { store } from '../store'
import type { Project } from '../types'
import AppShell from './AppShell.vue'
import {
  loadProjects,
  saveProjects,
  getStatusClass,
  toYoutubeId11,
  deleteProject,
  moveProject,
  saveProjectFromForm,
  parseParticipantConfig,
  serializeParticipantConfig,
  autoFillParticipantConfig,
  createDefaultParticipantConfig,
  type ProjectFormData,
} from '../api/projects'
import {
  fetchWithAbort,
  showToast,
  BACKEND_URL,
} from '../api/utils'
import {
  Folder, Crown, Plus, Pencil, Trash2, Film, ExternalLink, Save, Users,
  AlertTriangle, RefreshCw,
} from '@lucide/vue'

const selectedProject = ref<Project | null>(null)
const showParticipantTab = ref(false)
const participantConfig = ref(createDefaultParticipantConfig())
const editingIdx = ref(-1)
const loading = ref(true)
const error = ref(false)
const savingParticipant = ref(false)

const projectModalVisible = ref(false)
const projectFormTitle = ref('Добавить проект')
const formName = ref('')
const formVideo = ref('')
const formId = ref('')
const formStatus = ref('планируется')
const formVerifier = ref('')
const formComment = ref('')

onMounted(async () => {
  try {
    await Promise.all([loadProjects(), loadStaffRoles()])
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
})

async function loadStaffRoles() {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/staff`, {}, 'staff-list')
    if (res.ok) {
      const data = await res.json()
      store.staffRoles = Array.isArray(data) ? data : []
    }
  } catch { /* network error */ }
}

const projectDetailMousedown = ref(false)

function openProjectDetail(project: Project) {
  selectedProject.value = project
  document.body.classList.add('modal-open')
}

function closeProjectDetail() {
  selectedProject.value = null
  document.body.classList.remove('modal-open')
}

function openAddProject() {
  if (!store.isHost) { showToast('Только хост может добавлять проекты', 'error'); return }
  editingIdx.value = -1
  projectFormTitle.value = 'Добавить проект'
  formName.value = ''
  formVideo.value = ''
  formId.value = ''
  formStatus.value = 'планируется'
  formVerifier.value = ''
  formComment.value = ''
  projectModalVisible.value = true
  document.body.classList.add('modal-open')
}

function onEditProject(idx: number) {
  if (!store.isHost) { showToast('Только хост может редактировать проекты', 'error'); return }
  const project = store.projects[idx]
  if (!project) return
  editingIdx.value = idx
  projectFormTitle.value = 'Редактировать проект'
  formName.value = project.name || ''
  formVideo.value = project.videoId || ''
  formId.value = project.id || ''
  formStatus.value = project.status || 'планируется'
  formVerifier.value = project.verifier || ''
  formComment.value = project.comment || ''
  projectModalVisible.value = true
  document.body.classList.add('modal-open')
}

function closeProjectEditModal() {
  editingIdx.value = -1
  projectModalVisible.value = false
  document.body.classList.remove('modal-open')
}

function submitProjectForm() {
  const formData: ProjectFormData = {
    name: formName.value,
    videoId: formVideo.value,
    id: formId.value,
    comment: formComment.value,
    status: formStatus.value,
    verifier: formVerifier.value,
  }
  saveProjectFromForm(formData, editingIdx.value)
  closeProjectEditModal()
}

function openParticipantTabFromEditModal() {
  const idx = editingIdx.value
  if (idx !== -1) {
    selectedProject.value = store.projects[idx]
    openParticipantTab()
  }
}

function onProjectDetailMousedown(e: MouseEvent) {
  projectDetailMousedown.value = e.target === e.currentTarget
}

function onProjectDetailMouseup(e: MouseEvent) {
  if (projectDetailMousedown.value && e.target === e.currentTarget) {
    closeProjectDetail()
  }
  projectDetailMousedown.value = false
}

function openParticipantTab() {
  if (!selectedProject.value) return
  if (!store.isHost) { showToast('Только хост может редактировать участников', 'error'); return }
  const config = parseParticipantConfig(selectedProject.value)
  if (config.parts.length === 0) {
    const auto = autoFillParticipantConfig()
    config.parts = auto.parts
    config.host = auto.host
  }
  participantConfig.value = reactiveClone(config)
  initParticipantTabState()
  showParticipantTab.value = true
  document.body.classList.add('modal-open')
}

function closeParticipantTab() {
  showParticipantTab.value = false
}

function reactiveClone<T>(obj: T): T {
  return JSON.parse(JSON.stringify(obj))
}

function addPart() {
  participantConfig.value.parts.push({ gp: [], deco: [], transition: '' })
}

function removePart(index: number) {
  participantConfig.value.parts.splice(index, 1)
}

function addPlaytestField() {
  if (!participantConfig.value.playtest) participantConfig.value.playtest = []
  participantConfig.value.playtest.push('')
}

function removePlaytestField(index: number) {
  participantConfig.value.playtest.splice(index, 1)
}

function addVerifierField() {
  if (!participantConfig.value.verifier) participantConfig.value.verifier = []
  participantConfig.value.verifier.push('')
}

function removeVerifierField(index: number) {
  participantConfig.value.verifier.splice(index, 1)
}

function addMergerField() {
  if (!participantConfig.value.merger) participantConfig.value.merger = []
  participantConfig.value.merger.push('')
}

function removeMergerField(index: number) {
  participantConfig.value.merger.splice(index, 1)
}

function addMerger2Field() {
  if (!participantConfig.value.merger2) participantConfig.value.merger2 = []
  participantConfig.value.merger2.push('')
}

function removeMerger2Field(index: number) {
  participantConfig.value.merger2.splice(index, 1)
}

const showEndScreen = ref(false)

function toggleEndScreen() {
  showEndScreen.value = !showEndScreen.value
  if (!showEndScreen.value) {
    participantConfig.value.endScreen = []
  } else {
    if (!participantConfig.value.endScreen) participantConfig.value.endScreen = []
  }
}

const showShowcaser = ref(false)

function toggleShowcaser() {
  showShowcaser.value = !showShowcaser.value
  if (!showShowcaser.value) {
    participantConfig.value.showcaser = ''
  }
}

const showSoloGp = ref(false)

function toggleSoloGp() {
  showSoloGp.value = !showSoloGp.value
  if (!showSoloGp.value) {
    participantConfig.value.soloGp = ''
    participantConfig.value.parts.forEach(p => {
      p.gp = []
    })
  } else {
    participantConfig.value.soloGp = ''
  }
}

function initParticipantTabState() {
  const cfg = participantConfig.value
  showEndScreen.value = !!(cfg.endScreen && cfg.endScreen.length > 0)
  showShowcaser.value = !!cfg.showcaser
  showSoloGp.value = !!cfg.soloGp
}

async function saveParticipantConfig() {
  if (!selectedProject.value) return
  if (!store.isHost) { showToast('Только хост может сохранять участников', 'error'); return }
  const currentProject = selectedProject.value
  const proj = store.projects.find(p => p.id === currentProject.id)
  if (!proj) return
  proj.participants = serializeParticipantConfig(participantConfig.value)
  savingParticipant.value = true
  try {
    await saveProjects(store.projects)
    await loadProjects()
    selectedProject.value = store.projects.find(p => p.id === proj.id) ?? null
    showParticipantTab.value = false
    showToast('Участники сохранены!', 'success')
  } catch (e) {
    showToast((e instanceof Error ? e.message : 'Ошибка сохранения участников'), 'error')
  } finally {
    savingParticipant.value = false
  }
}

async function reloadProjects() {
  error.value = false
  loading.value = true
  try {
    await Promise.all([loadProjects(), loadStaffRoles()])
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
}

function getRoleByName(name: string) {
  return (store.staffRoles || []).find(r => r.name === name)
}

function roleColor(name: string) {
  const role = getRoleByName(name)
  return role?.color || null
}

function getColoredLabel(label: string) {
  const color = roleColor(label)
  return color ? { color } : {}
}

function stringifyMulti(value: unknown) {
  if (!value) return ''
  if (Array.isArray(value)) return value.filter(Boolean).join(' & ')
  return ''
}

function updateMultiField(arr: string[], str: string) {
  arr.length = 0
  str.split('&').forEach(s => {
    const t = s.trim()
    if (t) arr.push(t)
  })
}

function renderParticipants(participants: string[]) {
  if (!participants || participants.length === 0) return []
  const config = parseParticipantConfig({ participants })
  const items = []
  if (config.host) items.push({ name: config.host, role: 'HOST', color: roleColor('HOST') })
  config.parts.forEach((part) => {
    ;(part.gp || []).forEach(g => {
      if (g) items.push({ name: g, role: config.fxMode ? 'FX' : 'GP', color: config.fxMode ? roleColor('FX') : roleColor('GP') })
    })
    ;(part.deco || []).forEach(d => {
      if (d) items.push({ name: d, role: config.fxMode ? 'FX' : 'DECO', color: config.fxMode ? roleColor('FX') : roleColor('DECO') })
    })
    if (part.transition) items.push({ name: part.transition, role: 'TRANSITION', color: roleColor('TRANSITION') })
  })
  ;(config.endScreen || []).forEach(e => {
    if (e) items.push({ name: e, role: 'END SCREEN', color: roleColor('END SCREEN') || roleColor('DECO') })
  })
  ;(config.playtest || []).forEach(p => {
    if (p) items.push({ name: p, role: 'PLAYTEST', color: roleColor('PLAYTEST') })
  })
  ;(config.verifier || []).forEach(v => {
    if (v) items.push({ name: v, role: 'VERIFIER', color: roleColor('VERIFIER') })
  })
  ;(config.merger || []).forEach(m => {
    if (m) items.push({ name: m, role: 'MERGER', color: roleColor('MERGER') })
  })
  ;(config.merger2 || []).forEach(m => {
    if (m) items.push({ name: m, role: 'MERGER', color: roleColor('MERGER') })
  })
  if (config.showcaser) items.push({ name: config.showcaser, role: 'SHOWCASER', color: roleColor('SHOWCASER') })
  if (config.soloGp) items.push({ name: config.soloGp, role: 'SOLO GP', color: roleColor('GP') })
  return items
}
</script>

<template>
  <AppShell>
    <template #brand>
      <span class="header-logo"><Folder :size="20" /></span>
      <div class="header-title">
        <h1>Проекты SMLT</h1>
        <span class="header-subtitle">Уровни и коллабы</span>
      </div>
    </template>
    <template #actions>
    </template>
  </AppShell>

  <main class="app-main">
    <div class="container">
      <div v-if="store.isHost" class="admin-panel">
        <div class="admin-panel-header"><Crown :size="16" /> Управление проектами</div>
        <div class="admin-panel-content">
          <button class="btn btn-primary" @click="openAddProject()"><Plus :size="16" /> Добавить проект</button>
        </div>
      </div>

      <section class="info-section" style="padding-top:0">
        <div class="info-card info-card-highlighted">
          <p class="info-card-description">Здесь размещаются коллабы от SMLT. Каждый проект содержит информацию о статусе, участниках и ссылке на видео.</p>
        </div>
      </section>

      <section class="status-legend" style="margin-bottom:var(--spacing-lg)">
        <div class="status-legend-row">
          <span class="project-status status-ready">Готов</span>
          <span class="project-status status-verifying">В процессе верифа</span>
          <span class="project-status status-building">В процессе постройки</span>
          <span class="project-status status-planned">Планируется</span>
          <span class="project-status status-frozen">Заморожен</span>
          <span class="project-status status-dead">Мёртв</span>
        </div>
      </section>

      <div v-if="loading && store.projects.length === 0" class="admin-grid-full">
        <div class="projects-grid">
          <div v-for="i in 4" :key="i" class="skeleton-card"></div>
        </div>
        <div class="loading-state" style="padding:var(--spacing-md)">
          <div class="spinner"></div>
          <div class="loading-text">Загрузка проектов...</div>
        </div>
      </div>

      <div v-else-if="error" class="loading-state">
        <div class="error-icon"><AlertTriangle :size="48" /></div>
        <div class="error-text">Не удалось загрузить проекты</div>
        <button class="btn btn-primary" @click="reloadProjects"><RefreshCw :size="16" /> Повторить</button>
      </div>

      <TransitionGroup name="list" tag="div" class="projects-grid" id="projectsGrid">
        <div v-for="(project, idx) in store.projects" :key="project.id || idx" class="project-card project-card-clickable" @click="openProjectDetail(project)">
          <template v-if="toYoutubeId11(project.videoId)">
            <div class="project-video">
              <iframe :src="`https://www.youtube.com/embed/${toYoutubeId11(project.videoId)}?rel=0`" frameborder="0" allowfullscreen loading="lazy"
                allow="accelerometer;clipboard-write;encrypted-media;gyroscope;picture-in-picture;web-share"
                referrerpolicy="strict-origin-when-cross-origin"
></iframe>
            </div>
          </template>
          <template v-else>
            <div class="project-video">
              <div class="project-video-placeholder"><Film :size="48" /></div>
            </div>
          </template>
          <div class="project-content">
            <h3 class="project-title">{{ project.name || `Проект #${idx + 1}` }}</h3>
            <div class="project-info">
              <div class="project-info-item">
                <span class="project-info-label">Статус:</span>
                <span class="project-status" :class="getStatusClass(project.status)">{{ project.status || 'планируется' }}</span>
              </div>
              <div class="project-info-item">
                <span class="project-info-label">Участников:</span>
                <span class="project-info-value">{{ (project.participants || []).length }}</span>
              </div>
            </div>
            <div v-if="store.isHost" class="project-actions" @click.stop>
              <button class="btn btn-secondary btn-sm" @click="moveProject(idx, 'up')">↑</button>
              <button class="btn btn-secondary btn-sm" @click="moveProject(idx, 'down')">↓</button>
              <button class="btn btn-secondary btn-sm" @click="onEditProject(idx)"><Pencil :size="14" /> Редактировать</button>
              <button class="btn btn-danger btn-sm" @click="deleteProject(idx)"><Trash2 :size="14" /> Удалить</button>
            </div>
          </div>
        </div>
        <div v-if="!loading && store.projects.length === 0" class="empty-state admin-grid-full">
          <div class="empty-state-icon"><Folder :size="48" /></div>
          <p>Проектов пока нет</p>
          <p class="no-data-text">Создайте первый проект, чтобы начать</p>
        </div>
      </TransitionGroup>
    </div>
  </main>

  <Teleport to="body">
    <div class="modal-overlay" :class="{ active: projectModalVisible }">
      <div class="modal modal-lg">
        <div class="modal-header">
          <div class="modal-title"><Folder :size="16" /> {{ projectFormTitle }}</div>
          <button class="modal-close" @click="closeProjectEditModal()">✕</button>
        </div>
        <div class="modal-body">
          <form @submit.prevent="submitProjectForm">
            <div class="form-group">
              <label>Название:</label>
              <input type="text" class="form-input" placeholder="Название проекта" v-model="formName">
            </div>
            <div class="form-group">
              <label>Видео (YouTube ID или ссылка):</label>
              <input type="text" class="form-input" placeholder="https://youtube.com/watch?v=..." v-model="formVideo">
            </div>
            <div class="form-row">
              <div class="form-group">
                <label>ID:</label>
                <input type="text" class="form-input" placeholder="Уникальный ID (оставьте пустым для генерации)" v-model="formId">
              </div>
              <div class="form-group">
                <label>Статус:</label>
                <select class="form-select" v-model="formStatus">
                  <option value="планируется">Планируется</option>
                  <option value="в процессе постройки">В процессе постройки</option>
                  <option value="в процессе верифа">В процессе верифа</option>
                  <option value="готов">Готов</option>
                  <option value="заморожен">Заморожен</option>
                  <option value="мёртв">Мёртв</option>
                </select>
              </div>
            </div>
            <div class="form-group">
              <label>Верифнут:</label>
              <input type="text" class="form-input" placeholder="Кто верифер?" v-model="formVerifier">
            </div>
            <div class="form-group">
              <label>Комментарий:</label>
              <textarea class="form-textarea" placeholder="Комментарий к проекту" v-model="formComment"></textarea>
            </div>
            <div v-if="editingIdx !== -1" class="form-group">
              <button type="button" class="btn btn-primary btn-full-width" @click="openParticipantTabFromEditModal"><Users :size="16" /> Добавить участников</button>
            </div>
            <div class="modal-actions-row-spaced">
              <button type="button" class="btn btn-secondary" @click="closeProjectEditModal()">Отмена</button>
              <button type="submit" class="btn btn-primary"><Save :size="14" /> Сохранить</button>
            </div>
          </form>
        </div>
      </div>
    </div>

    <div v-if="selectedProject" class="modal-overlay active" @mousedown="onProjectDetailMousedown" @mouseup="onProjectDetailMouseup">
      <div class="modal modal-lg" @mousedown.stop @mouseup.stop>
        <div class="modal-header">
          <div class="modal-title">{{ selectedProject.name || 'Проект' }}</div>
          <button class="modal-close" @click="closeProjectDetail">✕</button>
        </div>
        <div class="modal-body">
          <div class="project-info">
            <div class="project-info-item">
              <span class="project-info-label">ID:</span>
              <span class="project-info-value">{{ selectedProject.id || '—' }}</span>
            </div>
            <div class="project-info-item">
              <span class="project-info-label">Статус:</span>
              <span class="project-status" :class="getStatusClass(selectedProject.status)">{{ selectedProject.status || 'планируется' }}</span>
            </div>
            <div class="project-info-item">
              <span class="project-info-label">Верифнут:</span>
              <span class="project-info-value">{{ selectedProject.verifier || '—' }}</span>
            </div>
            <div v-if="selectedProject.comment" class="project-info-item">
              <span class="project-info-label">Коммент:</span>
              <span class="project-info-value">{{ selectedProject.comment }}</span>
            </div>
          </div>
          <div class="project-participants" style="margin-top:var(--spacing-md)">
            <div class="project-participants-title">Участники:</div>
            <div class="project-participants-list project-participants-vertical">
              <template v-if="renderParticipants(selectedProject.participants).length > 0">
                <div v-for="(item, ei) in renderParticipants(selectedProject.participants)" :key="ei" class="participant-tag participant-tag-flex">
                  <strong>{{ item.name }}</strong>
                  <span v-if="item.role" class="participant-tag-role">
                    <span class="role" :style="item.color ? { color: item.color } : {}">{{ item.role }}</span>
                  </span>
                </div>
              </template>
              <div v-else class="no-participants-text">Нет участников</div>
            </div>
          </div>
          <template v-if="toYoutubeId11(selectedProject.videoId)">
            <div class="project-info-bordered">
                <a :href="`https://www.youtube.com/watch?v=${encodeURIComponent(toYoutubeId11(selectedProject.videoId) ?? '')}`" target="_blank" rel="noopener noreferrer" class="project-video-link"><ExternalLink :size="14" /> Открыть на YouTube</a>
            </div>
          </template>
        </div>
      </div>
    </div>

    <div v-if="store.isHost && showParticipantTab && selectedProject" class="modal-overlay active">
      <div class="modal modal-xl participant-modal">
        <div class="modal-header">
          <div class="modal-title">{{ selectedProject.name }} — Участники</div>
          <button class="modal-close" @click="closeParticipantTab">✕</button>
        </div>
        <div class="modal-body participant-modal-body">
          <div class="participant-layout">
            <div class="participant-left">
              <div class="participant-host">
                <span class="field-label" :style="getColoredLabel('HOST')">HOST</span>
                <input type="text" class="form-input participant-input" v-model="participantConfig.host" placeholder="HOST">
              </div>

              <div v-for="(part, i) in participantConfig.parts" :key="i" class="part-block">
                <div class="part-header">
                  <span>Парт {{ i + 1 }}</span>
                  <button class="btn btn-danger btn-xs" @click="removePart(i)" title="Удалить парт">✕</button>
                </div>
                <div class="part-field">
                  <span class="field-label" :style="getColoredLabel('GP')">GP</span>
                  <input type="text" class="form-input participant-input" :value="stringifyMulti(part.gp)" @input="updateMultiField(part.gp, ($event.target as HTMLInputElement).value)" placeholder="GP (разделитель &)">
                </div>
                <div class="part-field">
                  <span class="field-label" :style="getColoredLabel(participantConfig.fxMode ? 'FX' : 'DECO')">{{ participantConfig.fxMode ? 'FX' : 'DECO' }}</span>
                  <input type="text" class="form-input participant-input" :value="stringifyMulti(part.deco)" @input="updateMultiField(part.deco, ($event.target as HTMLInputElement).value)" placeholder="DECO (разделитель &)">
                </div>
                <div class="part-transition">
                  <span class="field-label">TRANSITION</span>
                  <input type="text" class="form-input participant-input" v-model="part.transition" placeholder="TRANSITION">
                </div>
              </div>

              <button class="btn btn-secondary btn-sm participant-add-btn" @click="addPart"><Plus :size="14" /> Добавить парт</button>

              <div class="part-block end-screen-block" v-if="showEndScreen">
                <div class="part-header">
                  <span>END SCREEN</span>
                </div>
                <div class="part-field">
                  <span class="field-label" :style="getColoredLabel('DECO')">END SCREEN</span>
                  <input type="text" class="form-input participant-input" :value="stringifyMulti(participantConfig.endScreen)" @input="updateMultiField(participantConfig.endScreen, ($event.target as HTMLInputElement).value)" placeholder="END SCREEN (разделитель &)">
                </div>
              </div>
              <button v-if="!showEndScreen" class="btn btn-deco btn-sm participant-add-btn" @click="toggleEndScreen"><Plus :size="14" /> Добавить END SCREEN</button>
              <button v-else class="btn btn-deco btn-sm participant-add-btn" @click="toggleEndScreen">✕ Убрать END SCREEN</button>
            </div>

            <div class="participant-right">
              <div class="participant-toggles">
                <label class="toggle-label">
                  <input type="checkbox" v-model="participantConfig.fxMode"> Режим FX
                </label>
                <label class="toggle-label">
                  <input type="checkbox" v-model="showSoloGp" @change="toggleSoloGp"> Соло GP
                </label>
              </div>

              <div v-if="showSoloGp" class="right-section">
                <span class="field-label" :style="getColoredLabel('GP')">SOLO GP</span>
                <input type="text" class="form-input participant-input" v-model="participantConfig.soloGp" placeholder="SOLO GP">
              </div>

              <div class="right-section">
                <div class="right-section-header">
                  <button class="btn btn-secondary btn-xs" @click="addPlaytestField"><Plus :size="14" /> PLAYTEST</button>
                </div>
                <div v-for="(_, i) in participantConfig.playtest" :key="'pt'+i" class="right-field-row">
                  <span class="field-label">PLAYTEST</span>
                  <input type="text" class="form-input participant-input" v-model="participantConfig.playtest[i]" placeholder="PLAYTEST">
                  <button class="btn btn-danger btn-xs" @click="removePlaytestField(i)">✕</button>
                </div>
              </div>

              <div class="right-section">
                <div class="right-section-header">
                  <button class="btn btn-secondary btn-xs" @click="addMergerField"><Plus :size="14" /> MERGER</button>
                </div>
                <div v-for="(_, i) in participantConfig.merger" :key="'mg'+i" class="right-field-row">
                  <span class="field-label">MERGER</span>
                  <input type="text" class="form-input participant-input" v-model="participantConfig.merger[i]" placeholder="MERGER">
                  <button class="btn btn-danger btn-xs" @click="removeMergerField(i)">✕</button>
                </div>
              </div>

              <div class="right-section">
                <div class="right-section-header">
                  <button v-if="!showShowcaser" class="btn btn-secondary btn-xs" @click="toggleShowcaser"><Plus :size="14" /> SHOWCASER</button>
                  <button v-else class="btn btn-danger btn-xs" @click="toggleShowcaser">✕ SHOWCASER</button>
                </div>
                <div v-if="showShowcaser" class="right-field-row">
                  <span class="field-label">SHOWCASER</span>
                  <input type="text" class="form-input participant-input" v-model="participantConfig.showcaser" placeholder="SHOWCASER">
                </div>
              </div>

              <div class="right-section">
                <div class="right-section-header">
                  <button class="btn btn-secondary btn-xs" @click="addMerger2Field"><Plus :size="14" /> MERGER</button>
                </div>
                <div v-for="(_, i) in participantConfig.merger2" :key="'mg2'+i" class="right-field-row">
                  <span class="field-label">MERGER</span>
                  <input type="text" class="form-input participant-input" v-model="participantConfig.merger2[i]" placeholder="MERGER">
                  <button class="btn btn-danger btn-xs" @click="removeMerger2Field(i)">✕</button>
                </div>
              </div>

              <div class="right-section">
                <div class="right-section-header">
                  <button class="btn btn-secondary btn-xs" @click="addVerifierField"><Plus :size="14" /> VERIFIER</button>
                </div>
                <div v-for="(_, i) in participantConfig.verifier" :key="'vr'+i" class="right-field-row">
                  <span class="field-label">VERIFIER</span>
                  <input type="text" class="form-input participant-input" v-model="participantConfig.verifier[i]" placeholder="VERIFIER">
                  <button class="btn btn-danger btn-xs" @click="removeVerifierField(i)">✕</button>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" @click="closeParticipantTab">Отмена</button>
          <button class="btn btn-primary" @click="saveParticipantConfig" :disabled="savingParticipant"><Save :size="14" /> {{ savingParticipant ? 'Сохранение...' : 'Сохранить участников' }}</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
