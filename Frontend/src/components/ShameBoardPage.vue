<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { store } from '../store'
import {
  loadShameBoard,
  checkShameBoard,
  saveShameReason,
  deleteShameEntry,
  syncShameBoard,
  addManualEntry,
  type ShameBoardEntry,
} from '../api/shame-board'
import AppShell from './AppShell.vue'
import { showToast } from '../api/utils'
import {
  Skull, Crown, RefreshCw, AlertTriangle, Pencil, Trash2, UserX, Plus,
} from '@lucide/vue'

const loading = ref(true)
const entries = ref<ShameBoardEntry[]>([])
const checking = ref(false)
const syncing = ref(false)
const editModalOpen = ref(false)
const editingEntry = ref<ShameBoardEntry | null>(null)
const editReason = ref('')
const notifications = ref<{ discordId: string; username: string }[]>([])
const notificationPanelOpen = ref(false)
const addModalOpen = ref(false)
const addUsername = ref('')
const addDiscordId = ref('')
const addReason = ref('')

const totalOnBoard = computed(() => entries.value.length)
const entriesWithReason = computed(() => entries.value.filter(e => e.reason).length)
const entriesWithoutReason = computed(() => entries.value.filter(e => !e.reason).length)

onMounted(async () => {
  entries.value = await loadShameBoard()
  loading.value = false
  if (store.isHost) {
    checkForNewMembers()
  }
})

async function checkForNewMembers() {
  checking.value = true
  const result = await checkShameBoard()
  if (result && result.newMembers && result.newMembers.length > 0) {
    notifications.value = result.newMembers
    notificationPanelOpen.value = true
    showToast(`Найдено ${result.newMembers.length} новых участников на Доске позора!`, 'error')
  }
  checking.value = false
}

async function doSync() {
  syncing.value = true
  const result = await syncShameBoard()
  if (result) {
    if (result.newCount > 0) {
      showToast(`Добавлено ${result.newCount} новых записей`, 'success')
      notifications.value = (result.added || []).map(a => ({ discordId: a.discordId, username: a.username }))
      notificationPanelOpen.value = true
    } else {
      showToast('Новых участников не найдено', 'success')
    }
    entries.value = await loadShameBoard()
  }
  syncing.value = false
}

function openEditModal(entry: ShameBoardEntry) {
  editingEntry.value = entry
  editReason.value = entry.reason || ''
  editModalOpen.value = true
  document.body.classList.add('modal-open')
}

function closeEditModal() {
  editModalOpen.value = false
  editingEntry.value = null
  editReason.value = ''
  document.body.classList.remove('modal-open')
}

async function doSaveReason() {
  if (!editingEntry.value) return
  await saveShameReason(editingEntry.value.discordId, editReason.value)
  closeEditModal()
  entries.value = await loadShameBoard()
}

async function doDeleteEntry(entry: ShameBoardEntry) {
  if (!confirm(`Удалить ${entry.username} с Доски позора?`)) return
  await deleteShameEntry(entry.discordId)
  entries.value = await loadShameBoard()
}

function closeNotifications() {
  notificationPanelOpen.value = false
  notifications.value = []
}

function getAvatarUrl(entry: ShameBoardEntry): string {
  if (!entry.avatar) return ''
  return `https://cdn.discordapp.com/avatars/${entry.discordId}/${entry.avatar}.png?size=128`
}

function openAddModal() {
  addUsername.value = ''
  addDiscordId.value = ''
  addReason.value = ''
  addModalOpen.value = true
  document.body.classList.add('modal-open')
}

function closeAddModal() {
  addModalOpen.value = false
  document.body.classList.remove('modal-open')
}

async function doAddManual() {
  if (!addUsername.value.trim()) return
  const ok = await addManualEntry(addUsername.value.trim(), addDiscordId.value.trim(), addReason.value.trim())
  if (ok) {
    closeAddModal()
    entries.value = await loadShameBoard()
  }
}
</script>

<template>
  <AppShell>
    <template #brand>
      <span class="header-logo"><Skull :size="20" /></span>
      <div class="header-title">
        <h1>Доска позора</h1>
        <span class="header-subtitle">Discord интеграция</span>
      </div>
    </template>
  </AppShell>

  <main class="app-main">
    <div class="container">

      <div v-if="store.isHost" class="admin-panel">
        <div class="admin-panel-header"><Crown :size="16" /> Управление</div>
        <div class="admin-panel-content">
          <button class="btn btn-primary" @click="doSync" :disabled="syncing">
            <RefreshCw :size="16" :class="{ 'spin-icon': syncing }" /> Синхронизировать с Discord
          </button>
          <button class="btn btn-secondary" @click="checkForNewMembers" :disabled="checking">
            <AlertTriangle :size="16" /> Проверить новых
          </button>
          <button class="btn btn-secondary" @click="openAddModal">
            <Plus :size="16" /> Добавить вручную
          </button>
        </div>
      </div>

      <div class="stats-grid" style="margin-bottom:var(--spacing-lg)">
        <div class="stats-section">
          <h3><Skull :size="16" /> Статистика</h3>
          <div class="stats-grid-main admin-grid-3">
            <div class="stat-card">
              <div class="stat-value" style="color: #ff4444">{{ totalOnBoard }}</div>
              <div class="stat-label">На Доске позора</div>
            </div>
            <div class="stat-card">
              <div class="stat-value" style="color: #ffaa00">{{ entriesWithoutReason }}</div>
              <div class="stat-label">Без причины</div>
            </div>
            <div class="stat-card">
              <div class="stat-value" style="color: #44ff44">{{ entriesWithReason }}</div>
              <div class="stat-label">С причиной</div>
            </div>
          </div>
        </div>
      </div>

      <section class="info-section" style="padding-top:0">
        <div class="info-card info-card-highlighted">
          <p class="info-card-description">Доска позора автоматически подтягивает участников с соответствующей ролью из Discord сервера.</p>
          <p class="info-card-description">Хост может указать причину попадания каждого участника.</p>
        </div>
      </section>

      <div v-if="notificationPanelOpen && notifications.length > 0" class="shame-notifications-panel">
        <div class="shame-notifications-header">
          <AlertTriangle :size="18" />
          <span>Новые участники на Доске позора ({{ notifications.length }})</span>
          <button class="btn btn-secondary btn-sm" @click="closeNotifications">Закрыть</button>
        </div>
        <div class="shame-notifications-list">
          <div v-for="n in notifications" :key="n.discordId" class="shame-notification-item">
            <span class="shame-notification-username">{{ n.username }}</span>
            <span class="shame-notification-id">{{ n.discordId }}</span>
          </div>
        </div>
      </div>

      <div v-if="loading">
        <div class="projects-grid">
          <div v-for="i in 4" :key="i" class="skeleton-card"></div>
        </div>
        <div class="loading-state" style="padding:var(--spacing-md)">
          <div class="spinner"></div>
          <div class="loading-text">Загрузка Доски позора...</div>
        </div>
      </div>

      <TransitionGroup name="list" tag="div" class="projects-grid">
        <div v-for="entry in entries" :key="entry.discordId" class="project-card shame-card">
          <div class="shame-card-header">
            <div class="shame-avatar">
              <img v-if="getAvatarUrl(entry)" :src="getAvatarUrl(entry)" :alt="entry.username" class="shame-avatar-img" />
              <div v-else class="shame-avatar-placeholder"><UserX :size="24" /></div>
            </div>
            <div class="shame-card-info">
              <div class="shame-card-username">{{ entry.username }}</div>
              <div class="shame-card-id">ID: {{ entry.discordId }}</div>
            </div>
          </div>
          <div class="shame-card-reason">
            <div v-if="entry.reason" class="shame-reason-text">
              <span class="shame-reason-label">Причина:</span> {{ entry.reason }}
            </div>
            <div v-else class="shame-reason-empty">
              Причина не указана
            </div>
          </div>
          <div v-if="entry.addedAt" class="shame-card-date">
            Добавлен: {{ new Date(entry.addedAt).toLocaleDateString('ru-RU') }}
          </div>
          <div v-if="store.isHost" class="project-actions">
            <button class="btn btn-primary btn-sm" @click="openEditModal(entry)">
              <Pencil :size="14" /> {{ entry.reason ? 'Редактировать' : 'Указать причину' }}
            </button>
            <button class="btn btn-danger btn-sm" @click="doDeleteEntry(entry)">
              <Trash2 :size="14" /> Удалить
            </button>
          </div>
        </div>
      </TransitionGroup>

      <div v-if="!loading && entries.length === 0" class="staff-empty-state">
        <div class="staff-empty-icon"><Skull :size="48" /></div>
        <p>Доска позора пуста</p>
        <p class="no-data-text">Никто пока не добавлен</p>
      </div>
    </div>
  </main>

  <Teleport to="body">
    <div class="modal-overlay" :class="{ active: editModalOpen }" @mousedown.self="closeEditModal">
      <div class="modal" style="max-width: 520px;" @mousedown.stop @mouseup.stop>
        <div class="modal-header">
          <div class="modal-title"><Pencil :size="16" /> {{ editingEntry?.username ? `Причина для ${editingEntry.username}` : 'Причина' }}</div>
          <button class="modal-close" @click="closeEditModal">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>Причина попадания на Доску позора</label>
            <textarea
              v-model="editReason"
              class="form-input shame-reason-textarea"
              placeholder="Опишите за что этот участник попал на Доску позора..."
              rows="4"
              maxlength="500"
            ></textarea>
            <div class="shame-char-count">{{ editReason.length }}/500</div>
          </div>
          <div class="modal-actions-row">
            <button class="btn btn-primary btn-full-width" @click="doSaveReason">Сохранить</button>
          </div>
        </div>
      </div>
    </div>

    <div class="modal-overlay" :class="{ active: addModalOpen }" @mousedown.self="closeAddModal">
      <div class="modal" style="max-width: 520px;" @mousedown.stop @mouseup.stop>
        <div class="modal-header">
          <div class="modal-title"><Plus :size="16" /> Добавить участника вручную</div>
          <button class="modal-close" @click="closeAddModal">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>Имя участника *</label>
            <input
              v-model="addUsername"
              type="text"
              class="form-input"
              placeholder="Username из Discord"
              maxlength="64"
            />
          </div>
          <div class="form-group">
            <label>Discord ID <span style="color:var(--color-text-muted);font-weight:400">(необязательно)</span></label>
            <input
              v-model="addDiscordId"
              type="text"
              class="form-input"
              placeholder="123456789012345678"
              maxlength="64"
            />
          </div>
          <div class="form-group">
            <label>Причина попадания</label>
            <textarea
              v-model="addReason"
              class="form-input shame-reason-textarea"
              placeholder="Опишите за что этот участник попал на Доску позора..."
              rows="4"
              maxlength="500"
            ></textarea>
            <div class="shame-char-count">{{ addReason.length }}/500</div>
          </div>
          <div class="modal-actions-row">
            <button class="btn btn-primary btn-full-width" @click="doAddManual" :disabled="!addUsername.trim()">Добавить</button>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.shame-card {
  border-left: 3px solid #ff4444;
}
.shame-card-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  background: var(--color-surface-2);
  border-radius: var(--border-radius-md);
  border: 1px solid var(--color-border);
  margin-bottom: var(--spacing-sm);
}
.shame-avatar {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  overflow: hidden;
  flex-shrink: 0;
  background: var(--color-surface-3);
  display: flex;
  align-items: center;
  justify-content: center;
}
.shame-avatar-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.shame-avatar-placeholder {
  color: var(--color-text-muted);
  display: flex;
  align-items: center;
  justify-content: center;
}
.shame-card-info {
  flex: 1;
}
.shame-card-username {
  font-weight: 700;
  font-size: var(--font-size-lg);
  color: var(--color-text);
}
.shame-card-id {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  font-family: monospace;
}
.shame-card-reason {
  padding: var(--spacing-sm) var(--spacing-md);
}
.shame-reason-text {
  font-size: var(--font-size-sm);
  color: var(--color-text);
}
.shame-reason-label {
  font-weight: 600;
  color: #ff4444;
}
.shame-reason-empty {
  font-size: var(--font-size-sm);
  color: var(--color-text-muted);
  font-style: italic;
}
.shame-card-date {
  padding: 0 var(--spacing-md) var(--spacing-sm);
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}
.shame-notifications-panel {
  background: rgba(255, 0, 0, 0.1);
  border: 1px solid rgba(255, 0, 0, 0.3);
  border-radius: var(--border-radius-md);
  padding: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}
.shame-notifications-header {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  color: #ff4444;
  font-weight: 600;
  margin-bottom: var(--spacing-sm);
}
.shame-notifications-header .btn {
  margin-left: auto;
}
.shame-notifications-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}
.shame-notification-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: var(--color-surface-2);
  border-radius: var(--border-radius-sm);
}
.shame-notification-username {
  font-weight: 600;
  color: var(--color-text);
}
.shame-notification-id {
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  font-family: monospace;
}
.shame-reason-textarea {
  width: 100%;
  min-height: 100px;
  resize: vertical;
}
.shame-char-count {
  text-align: right;
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
  margin-top: var(--spacing-xs);
}
.spin-icon {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
