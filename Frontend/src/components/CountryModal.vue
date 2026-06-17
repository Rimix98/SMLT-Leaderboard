<script setup>
import { computed } from 'vue'
import { store } from '../store'
import { resolveCountry, CODE_TO_NAME, getFlagCode } from '../api/utils'
import { makeOverlayClose } from '../utils/modal'

const props = defineProps({ countryName: { type: String, default: null }, visible: { type: Boolean, default: false } })
const emit = defineEmits(['close'])

const countryPlayers = computed(() => {
  if (props.countryName === null) {
    return store.allPlayers.filter(p => !p.nationality)
      .sort((a, b) => (a.rank || 999999) - (b.rank || 999999))
  }
  const code = resolveCountry(props.countryName)
  if (!code) return []
  return store.allPlayers.filter(p => resolveCountry(p.nationality) === code)
    .sort((a, b) => (a.rank || 999999) - (b.rank || 999999))
})

const displayName = computed(() => {
  if (props.countryName === null) return 'Неизвестно'
  const code = resolveCountry(props.countryName)
  return code ? (CODE_TO_NAME[code] || code) : props.countryName
})

const flagCode = computed(() => {
  if (props.countryName === null) return null
  return getFlagCode(props.countryName)
})

const close = makeOverlayClose(() => emit('close'))
</script>

<template>
  <div class="modal-overlay" :class="{ active: visible }" @mousedown="close.onMousedown" @mouseup="close.onMouseup">
    <div class="modal" @mousedown.stop @mouseup.stop>
      <div class="modal-header">
        <div class="modal-title">
          <img v-if="flagCode" :src="`https://flagcdn.com/w20/${flagCode}.png`" :alt="flagCode.toUpperCase()" width="20" class="flag-img flag-inline">
          <span v-else>🌍</span>
          Топ игроков: {{ displayName }}
        </div>
        <button class="modal-close" @click="emit('close')">✕</button>
      </div>
      <div class="modal-body">
        <template v-if="countryPlayers.length > 0">
          <div v-for="(p, idx) in countryPlayers" :key="p.id ?? idx" style="display:flex;justify-content:space-between;padding:var(--spacing-sm);border-bottom:1px solid var(--color-border)">
            <span><strong>#{{ idx + 1 }}</strong> {{ p.name }}</span>
            <span style="color:var(--color-text-muted)">{{ p.score?.toFixed(2) || '—' }} pts · #{{ p.rank || '—' }}</span>
          </div>
        </template>
        <p v-else style="color: var(--color-text-muted);">Нет данных</p>
      </div>
    </div>
  </div>
</template>
