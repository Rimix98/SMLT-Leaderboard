<script setup>
import { inject, onMounted } from 'vue'
import { store, initTheme } from '../store'
import { refreshCsrfToken } from '../api/utils'
import AppShell from './AppShell.vue'
import {
  loadStaffRoles,
  loadStaffTiers,
  getTierConfig,
  showAddRoleModal,
  showEditRoleModal,
  deleteRole,
  moveRole,
  removeStaffPlayer,
  closeAddRoleModal,
  createRole,
  addPlayerFromRoleModal,
  roleModalSortByTiers,
  roleModalToggleTiers,
  closeEditPanel,
  editAddPlayer,
  closeAddStaffPlayerModal,
  addPlayerToRole,
} from '../api/staff'

const openInfoModal = inject('openInfoModal')

onMounted(async () => {
  initTheme()
  await refreshCsrfToken()
  await Promise.all([loadStaffRoles(), loadStaffTiers()])
})
</script>

<template>
  <AppShell page="staff">
    <template #brand>
      <span class="header-logo">👥</span>
      <div class="header-title">
        <h1>Персонал SMLT</h1>
        <span class="header-subtitle">Состав команды</span>
      </div>
    </template>
    <template #actions>
      <button class="btn btn-secondary btn-lg" @click="openInfoModal">ℹ️ Информация</button>
    </template>
  </AppShell>

  <main class="app-main">
    <div class="container">
      <div v-if="store.isHost" class="admin-panel">
        <div class="admin-panel-header">👑 Управление ролями</div>
        <div class="admin-panel-content">
          <button class="btn btn-primary" @click="showAddRoleModal">➕ Создать роль</button>
        </div>
      </div>

      <section class="info-section" style="padding-top:0">
        <div class="info-card" style="border-left:3px solid var(--color-secondary)">
          <p style="color:var(--color-text-secondary);line-height:1.7">Здесь отображаются стафф SMLT.</p>
          <p style="color:var(--color-text-secondary);line-height:1.7;margin-top:var(--spacing-sm)">Тир-система:</p>
          <div class="tier-legend">
            <span class="tier-legend-item"><span class="tier-square" style="background:#00ffff"></span> приоритет</span>
            <span class="tier-legend-item"><span class="tier-square" style="background:#540b6d"></span> основа</span>
            <span class="tier-legend-item"><span class="tier-square" style="background:#6d0b0d"></span> резерв</span>
            <span class="tier-legend-item"><span class="tier-square" style="background:#888888"></span> N/A</span>
          </div>
        </div>
      </section>

      <div id="staffLoadingState" class="loading-state" v-if="store.staffRoles.length === 0">
        <div class="spinner"></div>
        <div class="loading-text">Загрузка состава...</div>
      </div>

      <div class="projects-grid" id="staffRolesContainer">
        <div v-for="(role, roleIndex) in store.staffRoles" :key="roleIndex" class="project-card">
          <div class="staff-role-visual" :style="{ background: role.color || '#3b82f6' }">
            <span class="staff-role-visual-name">{{ role.name }}</span>
          </div>
          <div class="project-content">
            <div class="project-info">
              <div class="project-info-item">
                <span class="project-info-label">Роль:</span>
                <span class="project-info-value" :style="{ color: role.color, fontWeight: 700 }">{{ role.name }}</span>
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
                  <span style="color:var(--color-text-muted);font-size:var(--font-size-xs)">Нет игроков</span>
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
              <button class="btn btn-primary btn-sm" @click="showEditRoleModal(roleIndex)">✏️ Редактировать</button>
              <button class="btn btn-danger btn-sm" @click="deleteRole(roleIndex)">🗑️ Удалить роль</button>
            </div>
          </div>
        </div>
      </div>

      <div v-if="store.staffRoles.length === 0" class="staff-empty-state" id="staffEmptyState">
        <div style="font-size:3rem">👥</div>
        <p>Роли пока не созданы</p>
      </div>
    </div>
  </main>

  <Teleport to="body">
    <div id="addRoleModal" class="modal-overlay" ref="addRoleModalEl">
      <div class="modal">
        <div class="modal-header">
          <div class="modal-title" id="addRoleModalTitle">🆕 Новая роль</div>
          <button class="modal-close" @click="closeAddRoleModal">✕</button>
        </div>
        <div class="modal-body">
          <input type="hidden" id="editRoleIndex" value="-1">
          <div class="form-group">
            <label for="roleName">Название роли:</label>
            <input type="text" id="roleName" class="form-input" placeholder="Например: Администрация">
          </div>
          <div class="form-group">
            <label for="roleColor">Цвет роли:</label>
            <div class="role-color-picker">
              <div class="color-picker-row">
                <input type="color" id="roleColor" class="color-input" value="#3b82f6">
                <div class="color-hex-input-wrapper">
                  <span class="color-hex-prefix">#</span>
                  <input type="text" id="roleColorHex" class="form-input color-hex-input" placeholder="f1c40f" maxlength="6">
                </div>
              </div>
            </div>
          </div>
          <div id="rolePlayerSection" class="form-group" style="display:none;border-top:1px solid var(--color-border);padding-top:var(--spacing-md);margin-top:var(--spacing-md)">
            <input type="hidden" id="editRolePlayerIdx" value="-1">
            <div style="display:flex;gap:var(--spacing-xs);margin-bottom:var(--spacing-sm);flex-wrap:wrap">
              <input type="text" id="rolePlayerSearch" class="form-input" placeholder="🔍 Поиск участника..." style="flex:1;min-width:100px">
              <button class="btn btn-secondary btn-sm" @click="roleModalSortByTiers">📊 Сорт. по тирам</button>
              <button class="btn btn-secondary btn-sm" id="roleToggleTiersBtn" @click="roleModalToggleTiers">🎯 Тир: вкл</button>
            </div>
            <div id="rolePlayerList" style="margin-bottom:var(--spacing-md)"></div>
            <div style="display:flex;gap:var(--spacing-sm);flex-wrap:wrap">
              <input type="text" id="roleAddPlayerNickname" class="form-input" placeholder="Ник игрока" style="flex:1;min-width:120px">
              <input type="text" id="roleAddPlayerDiscord" class="form-input" placeholder="Discord" style="flex:1;min-width:120px">
              <button class="btn btn-primary btn-sm" id="roleAddPlayerBtn" @click="addPlayerFromRoleModal">➕ Добавить</button>
            </div>
          </div>
          <div style="display:flex;gap:var(--spacing-sm);margin-top:var(--spacing-md)">
            <button class="btn btn-secondary" @click="closeAddRoleModal">Отмена</button>
            <button class="btn btn-primary" id="createRoleBtn" @click="createRole">Создать</button>
          </div>
        </div>
      </div>
    </div>

    <div id="addPlayerModal" class="modal-overlay">
      <div class="modal">
        <div class="modal-header">
          <div class="modal-title" id="addPlayerModalTitle">👤 Добавить игрока</div>
          <button class="modal-close" @click="closeAddStaffPlayerModal">✕</button>
        </div>
        <div class="modal-body">
          <input type="hidden" id="addPlayerRoleIndex" value="-1">
          <div class="form-group">
            <label for="playerNickname">Ник игрока:</label>
            <input type="text" id="playerNickname" class="form-input" placeholder="Nickname">
          </div>
          <div class="form-group">
            <label for="playerDiscord">Discord (необязательно):</label>
            <input type="text" id="playerDiscord" class="form-input" placeholder="Username#0000 or username">
          </div>
          <div style="display:flex;gap:var(--spacing-sm);margin-top:var(--spacing-md)">
            <button class="btn btn-secondary" @click="closeAddStaffPlayerModal">Отмена</button>
            <button class="btn btn-primary" @click="addPlayerToRole">Добавить игрока</button>
          </div>
        </div>
      </div>
    </div>

    <div id="editPanelOverlay" class="edit-panel-overlay"></div>
    <div id="editPanel" class="edit-panel">
      <div class="edit-panel-header">
        <span>✏️ Редактировать</span>
        <button class="edit-panel-close" @click="closeEditPanel">✕</button>
      </div>
      <div class="edit-panel-body">
        <input type="hidden" id="editPlayerKey" value="">
        <div class="form-group">
          <label for="editPlayerNickname">Ник игрока:</label>
          <input type="text" id="editPlayerNickname" class="form-input" placeholder="Nickname">
        </div>
        <div class="form-group">
          <label for="editPlayerDiscord">Discord (необязательно):</label>
          <input type="text" id="editPlayerDiscord" class="form-input" placeholder="Username#0000 or username">
        </div>
        <div class="form-group">
          <label for="editPlayerRole">Роль:</label>
          <select id="editPlayerRole" class="form-input"></select>
        </div>
        <button class="btn btn-primary" id="editPanelSubmitBtn" @click="editAddPlayer">➕ Добавить игрока</button>
        <div class="edit-player-list">
          <h4>Игроки:</h4>
          <input type="text" id="editPlayerSearch" class="form-input" placeholder="🔍 Поиск игрока..." style="margin-bottom:var(--spacing-sm)">
          <div id="editPlayerList"></div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
