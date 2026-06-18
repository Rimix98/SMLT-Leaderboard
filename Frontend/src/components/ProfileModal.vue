<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import { store } from '../store'
import { resolveCountry, CODE_TO_NAME, getFlagCode } from '../api/utils'
import { makeOverlayClose } from '../utils/modal'
import { Globe, ExternalLink } from '@lucide/vue'

const props = defineProps({ playerIndex: { type: Number, default: -1 } })
const emit = defineEmits(['close'])

const player = computed(() => store.players[props.playerIndex])

const records = computed(() => {
  if (!player.value?.records) return []
  return player.value.records.filter(r => r.status === 'accepted' && r.level)
})

const score = computed(() => player.value?.score?.toFixed(2) || '—')
const rank = computed(() => player.value?.rank || '—')

const flagCode = computed(() => getFlagCode(player.value?.nationality))

const close = makeOverlayClose(() => emit('close'))
</script>

<template>
  <div class="modal-overlay" :class="{ active: playerIndex >= 0 }" @mousedown="close.onMousedown" @mouseup="close.onMouseup">
    <div class="modal" @mousedown.stop @mouseup.stop>
      <div class="modal-header">
        <div class="modal-title">
          <img v-if="flagCode" :src="`https://flagcdn.com/w20/${flagCode}.png`" :alt="flagCode.toUpperCase()" width="20" class="flag-img flag-inline">
          <span v-else><Globe :size="16" /></span>
          {{ player?.name || 'Профиль' }}
        </div>
        <button class="modal-close" @click="emit('close')">✕</button>
      </div>
      <div class="modal-body" v-if="player">
        <div class="profile-stats">
          <div class="profile-stat">
            <div class="profile-stat-value">{{ score }}</div>
            <div class="profile-stat-label">Очки</div>
          </div>
          <div class="profile-stat">
            <div class="profile-stat-value">#{{ rank }}</div>
            <div class="profile-stat-label">Глобальный топ</div>
          </div>
          <div class="profile-stat">
            <div class="profile-stat-value">{{ records.length }}</div>
            <div class="profile-stat-label">Уровней</div>
          </div>
        </div>

        <div v-if="player.hardest" class="profile-info-row">
          <span class="profile-info-label">Hardest:</span>
          <span class="profile-info-value">{{ player.hardest.level?.name || player.hardest }}</span>
        </div>

        <div class="profile-info-row">
          <span class="profile-info-label">Страна:</span>
          <span class="profile-info-value">
            <img v-if="flagCode" :src="`https://flagcdn.com/w20/${flagCode}.png`" :alt="flagCode.toUpperCase()" width="20" class="flag-img flag-inline">
            <span v-else><Globe :size="16" /></span>
            {{ player.nationality || 'Не указана' }}
          </span>
        </div>

        <div class="profile-records-section">
          <h4>Пройденные уровни ({{ records.length }})</h4>
          <div class="profile-records-list">
            <template v-if="records.length > 0">
              <div v-for="(r, i) in records" :key="i" class="record-item">
                <span class="record-demon">
                  {{ r.level?.name || 'Неизвестно' }}
                  <span class="record-placement">#{{ r.level?.placement ?? '?' }}</span>
                </span>
                <span class="record-progress" :class="{ 'progress-100': (r.percent ?? r.progress ?? 100) >= 100 }">
                  {{ r.percent ?? r.progress ?? 100 }}%
                </span>
              </div>
            </template>
            <div v-else class="no-records">Нет записей</div>
          </div>
        </div>

        <div class="profile-link">
          <a :href="`https://demonlist.org/profile/${encodeURIComponent(String(player.id))}/`" target="_blank" rel="noopener noreferrer">
            <ExternalLink :size="14" /> Показать аккаунт в Global Demonlist →
          </a>
        </div>
      </div>
    </div>
  </div>
</template>
