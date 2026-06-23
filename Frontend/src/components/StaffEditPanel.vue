<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { store } from '../store'
import { getPlayerTier, TIER_CONFIG, addPlayerToRoleApi, removePlayerFromRoleApi, saveStaffRoles, loadStaffTiers, setPlayerTier } from '../api/staff'
import type { TierKey } from '../types'
import { Pencil } from '@lucide/vue'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ close: [] }>()

const nickname = ref('')
const discord = ref('')
const selectedRoleIndex = ref('')
const editKey = ref('')
const searchQuery = ref('')
const submitting = ref(false)

watch(() => props.visible, (v) => {
  if (!v) return
  nickname.value = ''
  discord.value = ''
  selectedRoleIndex.value = ''
  editKey.value = ''
  searchQuery.value = ''
  nextTick(() => { const f = document.getElementById('epNickname'); if (f) f.focus() })
})

const allPlayers = computed(() => {
  const list = []
  const q = searchQuery.value.toLowerCase().trim()
  for (const role of store.staffRoles) {
    const ri = store.staffRoles.indexOf(role)
    for (const p of (role.players || [])) {
      if (q && !p.nickname.toLowerCase().includes(q)) continue
      list.push({ ...p, _roleIndex: ri, _playerIndex: role.players.indexOf(p), _roleName: role.name })
    }
  }
  return list
})

async function submit() {
  const name = nickname.value.trim()
  if (!name) return
  const disc = discord.value.trim()
  const ri = parseInt(selectedRoleIndex.value)
  if (isNaN(ri) || ri < 0 || ri >= store.staffRoles.length) return

  submitting.value = true
  try {
    if (editKey.value) {
      const [oldRoleIdx, playerIdx] = editKey.value.split(':').map(Number)
      const oldRole = store.staffRoles[oldRoleIdx]
      const role = store.staffRoles[ri]
      if (oldRoleIdx !== ri) {
        if (oldRole?.players) oldRole.players.splice(playerIdx, 1)
        if (!role.players) role.players = []
        role.players.push({ nickname: name, discord: disc })
      } else if (role?.players?.[playerIdx]) {
        role.players[playerIdx].nickname = name
        role.players[playerIdx].discord = disc
      }
      await saveStaffRoles()
      await loadStaffTiers()
      editKey.value = ''
    } else {
      await addPlayerToRoleApi(ri, name, disc)
    }
    nickname.value = ''
    discord.value = ''
    selectedRoleIndex.value = ''
  } finally {
    submitting.value = false
  }
}

function editPlayer(p: { nickname: string; discord?: string; _roleIndex: number; _playerIndex: number; _roleName: string }) {
  nickname.value = p.nickname
  discord.value = p.discord || ''
  selectedRoleIndex.value = String(p._roleIndex)
  editKey.value = `${p._roleIndex}:${p._playerIndex}`
}

async function removePlayer(p: { nickname: string; _roleIndex: number; _roleName: string }) {
  if (!confirm(`Удалить игрока «${p.nickname}» из роли «${p._roleName}»?`)) return
  await removePlayerFromRoleApi(p._roleIndex, p.nickname)
}

async function onTierClick(nickname: string, tier: TierKey) {
  await setPlayerTier(nickname, tier)
}
</script>

<template>
  <div class="edit-panel-overlay" :class="{ active: visible }" @click="emit('close')"></div>
  <div class="edit-panel" :class="{ open: visible }">
    <div class="edit-panel-header">
      <span><Pencil :size="16" /> Редактировать</span>
      <button class="edit-panel-close" @click="emit('close')">✕</button>
    </div>
    <div class="edit-panel-body">
      <div class="form-group">
        <label>Ник игрока:</label>
        <input type="text" id="epNickname" class="form-input" placeholder="Nickname" v-model="nickname" @keyup.enter="submit">
      </div>
      <div class="form-group">
        <label>Discord (необязательно):</label>
        <input type="text" class="form-input" placeholder="Username#0000 or username" v-model="discord" @keyup.enter="submit">
      </div>
      <div class="form-group">
        <label>Роль:</label>
        <select class="form-input" v-model="selectedRoleIndex">
          <option value="" disabled selected>Выберите роль...</option>
          <option v-for="(role, idx) in store.staffRoles" :key="idx" :value="idx">{{ role.name }}</option>
        </select>
      </div>
      <button class="btn btn-primary" @click="submit" :disabled="submitting">{{ submitting ? 'Сохранение...' : (editKey ? 'Сохранить' : 'Добавить игрока') }}</button>
      <div class="edit-player-list">
        <h4>Игроки:</h4>
        <input type="text" class="form-input" placeholder="Поиск игрока..." style="margin-bottom:var(--spacing-sm)" v-model="searchQuery">
        <div v-if="allPlayers.length === 0" style="color:var(--color-text-muted);font-size:var(--font-size-xs)">
          {{ searchQuery ? 'Ничего не найдено' : 'Нет игроков' }}
        </div>
        <div v-for="p in allPlayers" :key="`${p._roleIndex}-${p._playerIndex}`" class="edit-player-list-item">
          <div class="player-info">
            <span class="player-nickname">{{ p.nickname }}</span>
            <span class="player-role-name">{{ p._roleName }}</span>
          </div>
          <span v-for="(cfg, key) in TIER_CONFIG" :key="key"
            class="role-tier-square"
            :style="{ background: cfg.color, opacity: getPlayerTier(p.nickname) === key ? '1' : '0.25', outline: getPlayerTier(p.nickname) === key ? '2px solid var(--color-text-primary)' : 'none', outlineOffset: '1px' }"
            :title="cfg.label"
            @click="onTierClick(p.nickname, key)"
          ></span>
          <button class="player-edit-btn" title="Редактировать" @click="editPlayer(p)"><Pencil :size="14" /></button>
          <button class="player-remove-btn" title="Удалить" @click="removePlayer(p)">✕</button>
        </div>
      </div>
    </div>
  </div>
</template>
