<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { store } from '../store'
import { getFlagCode } from '../api/utils'
import { makeOverlayClose } from '../utils/modal'
import { getPlayerHistory, type PlayerHistoryEntry } from '../api/history'
import { Globe, ExternalLink, TrendingUp } from '@lucide/vue'

const props = withDefaults(defineProps<{ playerIndex?: number }>(), { playerIndex: -1 })
const emit = defineEmits<{ close: [] }>()

const player = computed(() => store.players[props.playerIndex])

const records = computed(() => {
  if (!player.value?.records) return []
  return player.value.records.filter(r => r.status === 'accepted' && r.level)
})

const score = computed(() => player.value?.score?.toFixed(2) || '—')
const rank = computed(() => player.value?.rank || '—')

const flagCode = computed(() => getFlagCode(player.value?.nationality))

const close = makeOverlayClose(() => emit('close'))

const history = ref<PlayerHistoryEntry[]>([])
const historyLoading = ref(false)

async function loadHistory() {
  if (!player.value?.name) return
  historyLoading.value = true
  try {
    history.value = await getPlayerHistory(player.value.name)
  } finally {
    historyLoading.value = false
  }
}

const chartData = computed(() => {
  if (history.value.length === 0) return null
  const sorted = [...history.value].sort((a, b) => a.date.localeCompare(b.date))
  const maxRank = Math.max(...sorted.map(h => h.rank), 1)
  const minRank = Math.min(...sorted.map(h => h.rank), 0)
  const range = maxRank - minRank || 1
  return sorted.map(h => ({
    ...h,
    barHeight: Math.max(4, ((maxRank - h.rank + 1) / range) * 100),
  }))
})

watch(() => props.playerIndex, (val) => {
  if (val >= 0) {
    history.value = []
    loadHistory()
  }
})
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

        <div class="profile-history-section" v-if="chartData && chartData.length > 0">
          <h4><TrendingUp :size="14" /> История ({{ chartData.length }} дней)</h4>
          <div class="history-chart">
            <div v-for="(entry, i) in chartData" :key="i" class="history-bar-wrapper" :title="`#${entry.rank} · ${entry.score.toFixed(1)} pts · ${entry.date}`">
              <div class="history-bar" :style="{ height: entry.barHeight + '%' }"></div>
              <span class="history-date">{{ entry.date.slice(5) }}</span>
            </div>
          </div>
          <div class="history-legend">
            <span class="history-legend-item">
              <span class="history-legend-dot" style="background: var(--color-secondary)"></span> Позиция (чем выше — тем лучше)
            </span>
          </div>
        </div>
        <div v-else-if="historyLoading" class="profile-history-loading">
          <span class="spinner" style="width:16px;height:16px"></span> Загрузка истории...
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

<style scoped>
.profile-history-section {
  margin-top: var(--spacing-md);
  padding-top: var(--spacing-md);
  border-top: 1px solid var(--color-border);
}
.profile-history-section h4 {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: var(--spacing-sm);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}
.history-chart {
  display: flex;
  align-items: flex-end;
  gap: 2px;
  height: 80px;
  padding: var(--spacing-xs) 0;
}
.history-bar-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-end;
  height: 100%;
  min-width: 0;
}
.history-bar {
  width: 100%;
  max-width: 20px;
  background: var(--color-secondary);
  border-radius: 2px 2px 0 0;
  opacity: 0.8;
  transition: opacity 0.15s;
  cursor: default;
}
.history-bar-wrapper:hover .history-bar {
  opacity: 1;
}
.history-date {
  font-size: 9px;
  color: var(--color-text-muted);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
  text-align: center;
}
.history-legend {
  margin-top: var(--spacing-xs);
  font-size: var(--font-size-xs);
  color: var(--color-text-muted);
}
.history-legend-item {
  display: flex;
  align-items: center;
  gap: 4px;
}
.history-legend-dot {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  display: inline-block;
}
.profile-history-loading {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: var(--spacing-md) 0;
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}
</style>
