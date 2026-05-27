<script setup>
import { computed, onMounted } from 'vue'
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
} from '../api/projects'
import { getRoleColor } from '../api/staff'

onMounted(async () => {
  initTheme()
  await refreshCsrfToken()
  await loadProjects()
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
      <button class="btn btn-secondary btn-lg" @click="openInfoModal">ℹ️ Информация</button>
    </template>
  </AppShell>

  <main class="app-main">
    <div class="container">
      <div v-if="store.isHost" class="admin-panel">
        <div class="admin-panel-header">👑 Управление проектами</div>
        <div class="admin-panel-content">
          <button class="btn btn-primary" @click="showAddProjectModal">➕ Добавить проект</button>
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
        <div v-for="(project, idx) in store.projects" :key="project.id || idx" class="project-card">
          <template v-if="toYoutubeId11(project.videoId)">
            <div class="project-video">
              <iframe :src="`https://www.youtube.com/embed/${toYoutubeId11(project.videoId)}?rel=0`" frameborder="0" allowfullscreen
                allow="accelerometer;clipboard-write;encrypted-media;gyroscope;picture-in-picture;web-share"
                referrerpolicy="strict-origin-when-cross-origin"></iframe>
            </div>
            <div style="padding:var(--spacing-xs) var(--spacing-md);background:var(--color-surface-2);text-align:center">
              <a :href="`https://www.youtube.com/watch?v=${encodeURIComponent(toYoutubeId11(project.videoId))}`" target="_blank" rel="noopener noreferrer"
                style="font-size:var(--font-size-xs);color:var(--color-secondary)">🔗 Открыть на YouTube</a>
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
                <span class="project-info-label">ID:</span>
                <span class="project-info-value">{{ project.id || '—' }}</span>
              </div>
              <div class="project-info-item">
                <span class="project-info-label">Статус:</span>
                <span class="project-status" :class="getStatusClass(project.status)">{{ project.status || 'планируется' }}</span>
              </div>
              <div class="project-info-item">
                <span class="project-info-label">Верифнут:</span>
                <span class="project-info-value">{{ project.verifier || '—' }}</span>
              </div>
              <div v-if="project.comment" class="project-info-item">
                <span class="project-info-label">Коммент:</span>
                <span class="project-info-value">{{ project.comment }}</span>
              </div>
            </div>
            <div class="project-participants">
              <div class="project-participants-title">Участники:</div>
              <div class="project-participants-list">
              <span v-for="(entry, ei) in parseParticipantRoles(project.participants)" :key="ei" class="participant-tag">
                {{ entry.name }}<template v-if="entry.type === 'dash'"> - <template v-for="(role, ri) in entry.roles" :key="ri"><span v-if="ri"> </span><span class="role" :style="role.color ? { color: role.color } : {}">{{ role.name }}</span></template></template><template v-else-if="entry.type === 'paren'"> - (<template v-for="(role, ri) in entry.roles" :key="ri"><span v-if="ri">, </span><span class="role" :style="role.color ? { color: role.color } : {}">{{ role.name }}</span></template>)</template>
              </span>
            </div>
            </div>
            <div v-if="store.isHost" class="project-actions">
              <button class="btn btn-secondary btn-sm" @click="moveProject(idx, 'up')">↑</button>
              <button class="btn btn-secondary btn-sm" @click="moveProject(idx, 'down')">↓</button>
              <button class="btn btn-secondary btn-sm" @click="editProject(idx)">✏️ Редактировать</button>
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
</template>
