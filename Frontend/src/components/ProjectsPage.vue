<script setup>
import { computed, onMounted, ref } from 'vue'
import { store, initTheme } from '../store'
import { refreshCsrfToken } from '../api/utils'
import AppShell from './AppShell.vue'
import {
  loadProjects,
  getStatusClass,
  toYoutubeId11,
  editProject,
  deleteProject,
  moveProject,
  showAddProjectModal,
  closeProjectModal,
  saveProject,
  addProjectParticipant,
} from '../api/projects'
import { getRoleColor, loadStaffRoles } from '../api/staff'

const selectedProject = ref(null)

onMounted(async () => {
  initTheme()
  await refreshCsrfToken()
  await Promise.all([loadProjects(), loadStaffRoles()])
})

function parseParticipantRoles(participants) {
  if (!participants || participants.length === 0) return []
  return participants.map(line => {
    const newMatch = line.match(/^(.+?)\s*-\s+(.+)$/)
    const oldMatch = !newMatch ? line.match(/^(.+?)\s*\((.+?)\)$/) : null
    if (newMatch) {
      const name = newMatch[1].trim()
      const roles = newMatch[2].split(/\s+/).filter(Boolean).map(r => ({
        name: r,
        color: getRoleColor(r)
      }))
      return { name, roles, type: 'dash' }
    } else if (oldMatch) {
      const name = oldMatch[1].trim()
      const roles = oldMatch[2].split(',').map(r => {
        const t = r.trim()
        return { name: t, color: getRoleColor(t) }
      })
      return { name, roles, type: 'paren' }
    }
    return { name: line, roles: [], type: 'plain' }
  })
}

const projectDetailMousedown = ref(false)

function openProjectDetail(project) {
  selectedProject.value = project
  document.body.classList.add('modal-open')
}

function closeProjectDetail() {
  selectedProject.value = null
  document.body.classList.remove('modal-open')
}

function openAddProject() {
  showAddProjectModal()
  document.body.classList.add('modal-open')
}

function onEditProject(idx) {
  editProject(idx)
  document.body.classList.add('modal-open')
}

function closeProjectEditModal() {
  closeProjectModal()
  document.body.classList.remove('modal-open')
}

function onProjectDetailMousedown(e) {
  projectDetailMousedown.value = e.target === e.currentTarget
}

function onProjectDetailMouseup(e) {
  if (projectDetailMousedown.value && e.target === e.currentTarget) {
    closeProjectDetail()
  }
  projectDetailMousedown.value = false
}
</script>

<template>
  <AppShell page="projects">
    <template #brand>
      <span class="header-logo">📁</span>
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
        <div class="admin-panel-header">👑 Управление проектами</div>
        <div class="admin-panel-content">
          <button class="btn btn-primary" @click="openAddProject()">➕ Добавить проект</button>
        </div>
      </div>

      <section class="info-section" style="padding-top:0">
        <div class="info-card" style="border-left:3px solid var(--color-secondary)">
          <p style="color:var(--color-text-secondary);line-height:1.7">Здесь размещаются коллабы от SMLT. Каждый проект содержит информацию о статусе, участниках и ссылке на видео.</p>
        </div>
      </section>

      <section class="status-legend" style="margin-bottom:var(--spacing-lg)">
        <div style="display:flex;flex-wrap:wrap;gap:var(--spacing-sm)">
          <span class="project-status status-ready">Готов</span>
          <span class="project-status status-verifying">В процессе верифа</span>
          <span class="project-status status-building">В процессе постройки</span>
          <span class="project-status status-planned">Планируется</span>
          <span class="project-status status-frozen">Заморожен</span>
          <span class="project-status status-dead">Мёртв</span>
        </div>
      </section>

      <div class="projects-grid" id="projectsGrid">
        <div v-for="(project, idx) in store.projects" :key="project.id || idx" class="project-card" style="cursor:pointer" @click="openProjectDetail(project)">
          <template v-if="toYoutubeId11(project.videoId)">
            <div class="project-video">
              <iframe :src="`https://www.youtube.com/embed/${toYoutubeId11(project.videoId)}?rel=0`" frameborder="0" allowfullscreen
                allow="accelerometer;clipboard-write;encrypted-media;gyroscope;picture-in-picture;web-share"
                referrerpolicy="strict-origin-when-cross-origin"></iframe>
            </div>
          </template>
          <template v-else>
            <div class="project-video">
              <div class="project-video-placeholder">🎬</div>
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
              <button class="btn btn-secondary btn-sm" @click="onEditProject(idx)">✏️ Редактировать</button>
              <button class="btn btn-danger btn-sm" @click="deleteProject(idx)">🗑️ Удалить</button>
            </div>
          </div>
        </div>
        <div v-if="store.projects.length === 0" class="empty-state" style="grid-column:1/-1">
          <div class="empty-state-icon">📁</div>
          <p>Проектов пока нет</p>
        </div>
      </div>
    </div>
  </main>

  <Teleport to="body">
    <div id="projectModal" class="modal-overlay">
      <div class="modal modal-lg">
        <div class="modal-header">
          <div class="modal-title" id="projectModalTitle">📁 Добавить проект</div>
          <button class="modal-close" @click="closeProjectEditModal()">✕</button>
        </div>
        <div class="modal-body">
          <form id="projectForm" @submit.prevent="saveProject">
            <input type="hidden" id="projectIndex" value="-1">
            <div class="form-group">
              <label for="projectName">Название:</label>
              <input type="text" id="projectName" class="form-input" placeholder="Название проекта">
            </div>
            <div class="form-group">
              <label for="projectVideo">Видео (YouTube ID или ссылка):</label>
              <input type="text" id="projectVideo" class="form-input" placeholder="https://youtube.com/watch?v=...">
            </div>
            <div class="form-row">
              <div class="form-group">
                <label for="projectId">ID:</label>
                <input type="text" id="projectId" class="form-input" placeholder="Уникальный ID (оставьте пустым для генерации)">
              </div>
              <div class="form-group">
                <label for="projectStatus">Статус:</label>
                <select id="projectStatus" class="form-select">
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
              <label for="projectVerifier">Верифнут:</label>
              <input type="text" id="projectVerifier" class="form-input" placeholder="Кто верифер?">
            </div>
            <div class="form-group">
              <label for="projectComment">Комментарий:</label>
              <textarea id="projectComment" class="form-textarea" placeholder="Комментарий к проекту"></textarea>
            </div>
            <div class="form-group">
              <label>Участники:</label>
              <div class="participant-builder">
                <div class="participant-add-row">
                  <div class="participant-search-wrapper">
                    <input type="text" id="participantSearchInput" class="form-input" placeholder="🔍 Поиск участника..." autocomplete="off">
                    <div id="participantSearchResults" class="participant-search-results"></div>
                  </div>
                  <div id="participantRoleTags" class="participant-role-tags"></div>
                  <button type="button" id="addParticipantBtn" class="btn btn-secondary btn-sm" @click="addProjectParticipant">➕ Добавить участника</button>
                </div>
                <div id="participantsPreview" class="participants-preview"></div>
              </div>
            </div>
            <div style="display:flex;gap:var(--spacing-sm);margin-top:var(--spacing-md)">
              <button type="button" class="btn btn-secondary" @click="closeProjectEditModal()">Отмена</button>
              <button type="submit" class="btn btn-primary">💾 Сохранить</button>
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
            <div class="project-participants-list" style="display:flex;flex-direction:column;gap:var(--spacing-xs)">
              <div v-for="(entry, ei) in parseParticipantRoles(selectedProject.participants)" :key="ei" class="participant-tag" style="display:flex;flex-wrap:wrap;gap:2px">
                <strong>{{ entry.name }}</strong>
                <template v-if="entry.type === 'dash'">
                  <span v-for="(role, ri) in entry.roles" :key="ri" style="margin-left:2px">
                    <span v-if="ri > 0" style="margin:0 2px">·</span>
                    <span class="role" :style="role.color ? { color: role.color } : {}">{{ role.name }}</span>
                  </span>
                </template>
                <template v-else-if="entry.type === 'paren'">
                  <span v-for="(role, ri) in entry.roles" :key="ri" style="margin-left:2px">
                    <span v-if="ri > 0">, </span>
                    <span class="role" :style="role.color ? { color: role.color } : {}">{{ role.name }}</span>
                  </span>
                </template>
              </div>
              <div v-if="!selectedProject.participants || selectedProject.participants.length === 0" style="color:var(--color-text-muted);font-size:var(--font-size-xs)">Нет участников</div>
            </div>
          </div>
          <template v-if="toYoutubeId11(selectedProject.videoId)">
            <div style="margin-top:var(--spacing-md);padding-top:var(--spacing-md);border-top:1px solid var(--color-border)">
              <a :href="`https://www.youtube.com/watch?v=${encodeURIComponent(toYoutubeId11(selectedProject.videoId))}`" target="_blank" rel="noopener noreferrer" style="color:var(--color-secondary)">🔗 Открыть на YouTube</a>
            </div>
          </template>
        </div>
      </div>
    </div>
  </Teleport>
</template>
