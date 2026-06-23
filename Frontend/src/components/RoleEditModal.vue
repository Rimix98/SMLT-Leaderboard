<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { store } from '../store'
import { getPlayerTier, TIER_CONFIG, sortRolePlayersByTiers, createRoleApi, updateRoleApi, removePlayerFromRoleApi, saveStaffRoles, setPlayerTier, toggleRoleTiers } from '../api/staff'
import type { TierKey } from '../types'
import { makeOverlayClose } from '../utils/modal'
import {
  Pencil, BarChart3, Target,
} from '@lucide/vue'

const props = withDefaults(defineProps<{ visible: boolean; roleIndex?: number }>(), { roleIndex: -1 })
const emit = defineEmits<{ close: [] }>()

const isEditing = computed(() => props.roleIndex >= 0)
const role = computed(() => isEditing.value ? store.staffRoles[props.roleIndex] : null)

const roleName = ref('')
const roleColor = ref('#3b82f6')
const playerSearch = ref('')
const addNickname = ref('')
const addDiscord = ref('')
const editPlayerIdx = ref(-1)
const submitting = ref(false)

const closeOverlay = makeOverlayClose(() => emit('close'))

watch(() => props.visible, (v) => {
  if (!v) return
  if (isEditing.value && role.value) {
    roleName.value = role.value.name
    roleColor.value = role.value.color || '#3b82f6'
  } else {
    roleName.value = ''
    roleColor.value = store.selectedRoleColor || '#3b82f6'
  }
  playerSearch.value = ''
  addNickname.value = ''
  addDiscord.value = ''
  editPlayerIdx.value = -1
  nextTick(() => { const f = document.getElementById('roleNameInput'); if (f) f.focus() })
})

const filteredPlayers = computed(() => {
  if (!role.value) return []
  const q = playerSearch.value.toLowerCase().trim()
  const players = role.value.players || []
  if (!q) return players.map((p, i) => ({ ...p, _idx: i }))
  return players.map((p, i) => ({ ...p, _idx: i })).filter(p => p.nickname.toLowerCase().includes(q))
})

const isTiersEnabled = computed(() => role.value?.tiersEnabled !== false)

async function submitRole() {
  const name = roleName.value.trim()
  if (!name) return
  const color = roleColor.value || '#3b82f6'
  submitting.value = true
  try {
    if (isEditing.value) {
      await updateRoleApi(props.roleIndex, name, color)
    } else {
      await createRoleApi(name, color)
    }
    emit('close')
  } finally {
    submitting.value = false
  }
}

function startEditPlayer(pIdx: number) {
  const p = role.value?.players?.[pIdx]
  if (!p) return
  addNickname.value = p.nickname
  addDiscord.value = p.discord || ''
  editPlayerIdx.value = pIdx
}

async function movePlayer(pIdx: number, dir: 'up' | 'down') {
  const r = role.value
  if (!r?.players) return
  const target = dir === 'down' ? pIdx + 1 : pIdx - 1
  if (target < 0 || target >= r.players.length) return
  ;[r.players[pIdx], r.players[target]] = [r.players[target], r.players[pIdx]]
  await saveStaffRoles()
}

async function removePlayer(nickname: string) {
  if (!isEditing.value) return
  if (!confirm(`Удалить игрока «${nickname}» из роли?`)) return
  await removePlayerFromRoleApi(props.roleIndex, nickname)
}

async function sortByTiers() {
  if (!isEditing.value || !role.value) return
  sortRolePlayersByTiers(role.value)
  await saveStaffRoles()
}

async function toggleTiers() {
  if (!isEditing.value) return
  await toggleRoleTiers(props.roleIndex)
}

async function onTierClick(nickname: string, tier: TierKey) {
  await setPlayerTier(nickname, tier)
}
</script>

<template>
  <div class="modal-overlay" :class="{ active: visible }" @mousedown="closeOverlay.onMousedown" @mouseup="closeOverlay.onMouseup">
    <div class="modal" @mousedown.stop @mouseup.stop>
      <div class="modal-header">
        <div class="modal-title">{{ isEditing ? 'Редактировать роль' : 'Новая роль' }}</div>
        <button class="modal-close" @click="emit('close')">✕</button>
      </div>
      <div class="modal-body">
        <div class="form-group">
          <label>Название роли:</label>
          <input type="text" id="roleNameInput" class="form-input" placeholder="Например: Администрация" v-model="roleName">
        </div>
        <div class="form-group">
          <label>Цвет роли:</label>
          <div class="role-color-picker">
            <div class="color-picker-row">
              <input type="color" class="color-input" v-model="roleColor">
              <div class="color-hex-input-wrapper">
                <span class="color-hex-prefix">#</span>
                <input type="text" class="form-input color-hex-input" placeholder="f1c40f" maxlength="6" :value="roleColor.replace('#', '')" @input="roleColor = '#' + ($event.target as HTMLInputElement).value">
              </div>
            </div>
          </div>
        </div>

        <div v-if="isEditing" class="form-group" style="border-top:1px solid var(--color-border);padding-top:var(--spacing-md);margin-top:var(--spacing-md)">
          <div style="display:flex;gap:var(--spacing-xs);margin-bottom:var(--spacing-sm);flex-wrap:wrap">
            <input type="text" class="form-input" placeholder="Поиск участника..." style="flex:1;min-width:100px" v-model="playerSearch">
            <button class="btn btn-secondary btn-sm" @click="sortByTiers"><BarChart3 :size="14" /> Сорт. по тирам</button>
            <button class="btn btn-secondary btn-sm" @click="toggleTiers"><Target :size="14" /> Тир: {{ isTiersEnabled ? 'вкл' : 'выкл' }}</button>
          </div>

          <div style="margin-bottom:var(--spacing-md)">
            <div v-if="filteredPlayers.length === 0" style="color:var(--color-text-muted);font-size:var(--font-size-xs)">
              {{ playerSearch ? 'Ничего не найдено' : 'Нет игроков' }}
            </div>
            <div v-for="p in filteredPlayers" :key="p._idx" class="edit-player-list-item">
              <div class="player-info">
                <span class="player-nickname">{{ p.nickname }}</span>
                <span v-if="p.discord" class="player-role-name">{{ p.discord }}</span>
              </div>
              <template v-if="isTiersEnabled">
                <span v-for="(cfg, key) in TIER_CONFIG" :key="key"
                  class="role-tier-square"
                  :style="{ background: cfg.color, opacity: getPlayerTier(p.nickname) === key ? '1' : '0.25', outline: getPlayerTier(p.nickname) === key ? '2px solid var(--color-text-primary)' : 'none', outlineOffset: '1px' }"
                  :title="cfg.label"
                  @click="onTierClick(p.nickname, key)"
                ></span>
              </template>
              <button class="player-edit-btn" title="Редактировать" @click="startEditPlayer(p._idx)"><Pencil :size="14" /></button>
              <button v-if="p._idx > 0" class="player-edit-btn" title="Вверх" @click="movePlayer(p._idx, 'up')">↑</button>
              <button v-if="p._idx < (role?.players?.length || 0) - 1" class="player-edit-btn" title="Вниз" @click="movePlayer(p._idx, 'down')">↓</button>
              <button class="player-remove-btn" title="Удалить" @click="removePlayer(p.nickname)">✕</button>
            </div>
          </div>
        </div>

        <div class="modal-actions-row-spaced">
          <button class="btn btn-secondary" @click="emit('close')">Отмена</button>
          <button class="btn btn-primary" @click="submitRole" :disabled="submitting">{{ submitting ? 'Сохранение...' : (isEditing ? 'Сохранить' : 'Создать') }}</button>
        </div>
      </div>
    </div>
  </div>
</template>
