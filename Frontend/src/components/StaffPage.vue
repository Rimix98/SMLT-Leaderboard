<script setup>
import { computed, onMounted, ref } from 'vue'
import { store } from '../store'
import { loadStaffRoles, loadStaffTiers, getTierConfig, getPlayerTier, removeStaffPlayer, deleteRole, moveRole } from '../api/staff'
import AppShell from './AppShell.vue'
import RoleEditModal from './RoleEditModal.vue'
import AddPlayerModal from './AddPlayerModal.vue'
import StaffEditPanel from './StaffEditPanel.vue'

const loading = ref(true)
const roleModalVisible = ref(false)
const roleModalIndex = ref(-1)
const addPlayerModalVisible = ref(false)
const addPlayerRoleIndex = ref(-1)
const editPanelVisible = ref(false)

const totalUniqueParticipants = computed(() => {
  const names = new Set()
  store.staffRoles.forEach(role => {
    ;(role.players || []).forEach(p => names.add(p.nickname))
  })
  return names.size
})

const totalRoles = computed(() => store.staffRoles.length)

const tierCounts = computed(() => {
  const counts = { priority: 0, base: 0, reserve: 0, na: 0 }
  store.staffRoles.forEach(role => {
    ;(role.players || []).forEach(p => {
      const tierKey = getPlayerTier(p.nickname)
      if (counts[tierKey] !== undefined) counts[tierKey]++
    })
  })
  return counts
})

onMounted(async () => {
  await Promise.all([loadStaffRoles(), loadStaffTiers()])
  loading.value = false
})

function openAddRole() {
  roleModalIndex.value = -1
  roleModalVisible.value = true
  document.body.classList.add('modal-open')
}
function openEditRole(idx) {
  roleModalIndex.value = idx
  roleModalVisible.value = true
  document.body.classList.add('modal-open')
}
function closeRoleModal() {
  roleModalVisible.value = false
  document.body.classList.remove('modal-open')
}

function openAddPlayer(idx) {
  addPlayerRoleIndex.value = idx
  addPlayerModalVisible.value = true
  document.body.classList.add('modal-open')
}
function closeAddPlayerModal() {
  addPlayerModalVisible.value = false
  document.body.classList.remove('modal-open')
}

function openEditPanel() {
  editPanelVisible.value = true
  document.body.style.overflow = 'hidden'
}
function closeEditPanel() {
  editPanelVisible.value = false
  document.body.style.overflow = ''
}
</script>

<template>
  <AppShell>
    <template #brand>
      <span class="header-logo">👥</span>
      <div class="header-title">
        <h1>Персонал SMLT</h1>
        <span class="header-subtitle">Состав команды</span>
      </div>
    </template>
  </AppShell>

  <main class="app-main">
    <div class="container">
      <div v-if="store.isHost" class="admin-panel">
        <div class="admin-panel-header">👑 Управление ролями</div>
        <div class="admin-panel-content">
          <button class="btn btn-primary" @click="openAddRole">➕ Создать роль</button>
          <button class="btn btn-secondary" @click="openEditPanel">✏️ Редактировать</button>
        </div>
      </div>

      <div class="stats-grid" style="margin-bottom:var(--spacing-lg)">
        <div class="stats-section">
          <h3>📊 Статистика</h3>
          <div class="stats-grid-main admin-grid-3">
            <div class="stat-card">
              <div class="stat-value">{{ totalUniqueParticipants }}</div>
              <div class="stat-label">Всего участников</div>
            </div>
            <div class="stat-card">
              <div class="stat-value">{{ totalRoles }}</div>
              <div class="stat-label">Ролей</div>
            </div>
            <div class="stat-card">
              <div class="stat-value" style="font-size:var(--font-size-sm)">{{ tierCounts.priority + tierCounts.base + tierCounts.reserve + tierCounts.na }}</div>
              <div class="stat-label">Участников (с учётом ролей)</div>
            </div>
          </div>
        </div>
        <div class="stats-section">
          <h3>🎯 По тирам</h3>
          <div class="stats-grid-main admin-grid-4">
            <div class="stat-card">
              <div class="stat-value tier-stat-priority">{{ tierCounts.priority }}</div>
              <div class="stat-label">Приоритет</div>
            </div>
            <div class="stat-card">
              <div class="stat-value tier-stat-base">{{ tierCounts.base }}</div>
              <div class="stat-label">Основа</div>
            </div>
            <div class="stat-card">
              <div class="stat-value tier-stat-reserve">{{ tierCounts.reserve }}</div>
              <div class="stat-label">Резерв</div>
            </div>
            <div class="stat-card">
              <div class="stat-value tier-stat-na">{{ tierCounts.na }}</div>
              <div class="stat-label">N/A</div>
            </div>
          </div>
        </div>
      </div>

      <section class="info-section" style="padding-top:0">
        <div class="info-card info-card-highlighted">
          <p class="info-card-description">Здесь отображаются стафф SMLT.</p>
          <p class="info-card-description">Тир-система:</p>
          <div class="tier-legend">
            <span class="tier-legend-item"><span class="tier-square" style="background:#00ffff"></span> приоритет</span>
            <span class="tier-legend-item"><span class="tier-square" style="background:#540b6d"></span> основа</span>
            <span class="tier-legend-item"><span class="tier-square" style="background:#6d0b0d"></span> резерв</span>
            <span class="tier-legend-item"><span class="tier-square" style="background:#888888"></span> N/A</span>
          </div>
        </div>
      </section>

      <div v-if="loading && store.staffRoles.length === 0">
        <div class="projects-grid">
          <div v-for="i in 4" :key="i" class="skeleton-card"></div>
        </div>
        <div class="loading-state" style="padding:var(--spacing-md)">
          <div class="spinner"></div>
          <div class="loading-text">Загрузка состава...</div>
        </div>
      </div>

      <TransitionGroup name="list" tag="div" class="projects-grid">
        <div v-for="(role, roleIndex) in store.staffRoles" :key="roleIndex" class="project-card">
          <div class="staff-role-visual" :style="{ background: role.color || '#3b82f6' }">
            <span class="staff-role-visual-name">{{ role.name }}</span>
          </div>
          <div class="project-content">
            <div class="project-info">
              <div class="project-info-item">
                <span class="project-info-label">Роль:</span>
                <span class="project-info-value role-color-bold" :style="{ color: role.color }">{{ role.name }}</span>
              </div>
              <div class="project-info-item">
                <span class="project-info-label">Участников:</span>
                <span class="project-info-value">{{ (role.players || []).length }}</span>
              </div>
            </div>
            <div class="project-participants">
              <div class="project-participants-title">Участники:</div>
              <div class="project-participants-list">
                <template v-if="(role.players || []).length === 0">
                  <span class="no-players-text">Нет игроков</span>
                </template>
                <div v-for="(player, pIdx) in role.players" :key="pIdx" class="staff-player-row">
                  <span class="participant-tag staff-player-tag">
                    <span class="nickname-glow">{{ player.nickname }}</span>
                    <span v-if="player.discord" class="staff-player-discord-inline">{{ player.discord }}</span>
                    <span v-if="role.tiersEnabled !== false"
                      class="staff-tier-dot"
                      :style="{ background: getTierConfig(player.nickname).color }"
                      :title="getTierConfig(player.nickname).label"
                    ></span>
                    <button v-if="store.isHost" class="staff-player-remove-tag" @click="removeStaffPlayer(roleIndex, pIdx)" title="Удалить игрока">✕</button>
                  </span>
                </div>
              </div>
            </div>
            <div v-if="store.isHost" class="project-actions">
              <button class="btn btn-secondary btn-sm" @click="moveRole(roleIndex, 'up')">↑</button>
              <button class="btn btn-secondary btn-sm" @click="moveRole(roleIndex, 'down')">↓</button>
              <button class="btn btn-primary btn-sm" @click="openAddPlayer(roleIndex)">➕ Игрок</button>
              <button class="btn btn-primary btn-sm" @click="openEditRole(roleIndex)">✏️ Редактировать</button>
              <button class="btn btn-danger btn-sm" @click="deleteRole(roleIndex)">🗑️ Удалить роль</button>
            </div>
          </div>
        </div>
      </TransitionGroup>

      <div v-if="!loading && store.staffRoles.length === 0" class="staff-empty-state">
        <div class="staff-empty-icon">👥</div>
        <p>Роли пока не созданы</p>
        <p class="no-data-text">Создайте первую роль, чтобы добавить участников</p>
      </div>
    </div>
  </main>

  <Teleport to="body">
    <RoleEditModal :visible="roleModalVisible" :role-index="roleModalIndex" @close="closeRoleModal" />
    <AddPlayerModal :visible="addPlayerModalVisible" :role-index="addPlayerRoleIndex" @close="closeAddPlayerModal" />
    <StaffEditPanel :visible="editPanelVisible" @close="closeEditPanel" />
  </Teleport>
</template>
