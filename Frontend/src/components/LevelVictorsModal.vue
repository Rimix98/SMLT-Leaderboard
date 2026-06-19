<script setup lang="ts">
import { computed } from 'vue'
import { store } from '../store'
import { getFlagCode } from '../api/utils'
import { makeOverlayClose } from '../utils/modal'
import { Trophy, Globe } from '@lucide/vue'

const props = withDefaults(defineProps<{ levelId?: string | number | null; visible?: boolean }>(), { levelId: null, visible: false })
const emit = defineEmits<{ close: [] }>()

const levelData = computed(() => {
  if (props.levelId == null) return null
  return store.levels.levelData?.get(String(props.levelId)) || null
})

const close = makeOverlayClose(() => emit('close'))
</script>

<template>
  <div class="modal-overlay" :class="{ active: visible }" @mousedown="close.onMousedown" @mouseup="close.onMouseup">
    <div class="modal" @mousedown.stop @mouseup.stop>
      <div class="modal-header">
        <div class="modal-title"><Trophy :size="16" /> {{ levelData?.name || 'Уровень' }} #{{ levelData?.placement }}</div>
        <button class="modal-close" @click="emit('close')">✕</button>
      </div>
      <div class="modal-body">
        <template v-if="levelData && levelData.victors.length > 0">
          <div v-for="(victor, idx) in levelData.victors" :key="victor.id" class="level-victors-list" style="display:flex;justify-content:space-between;padding:var(--spacing-sm);border-bottom:1px solid var(--color-border)">
            <span>
              <strong>#{{ idx + 1 }}</strong>
              <img v-if="getFlagCode(victor.nationality)" :src="`https://flagcdn.com/w20/${getFlagCode(victor.nationality)}.png`" :alt="getFlagCode(victor.nationality).toUpperCase()" width="20" class="flag-img flag-inline">
              <span v-else><Globe :size="16" /></span>
              {{ victor.name }}
            </span>
          </div>
        </template>
        <p v-else style="color: var(--color-text-muted);">Нет викторов</p>
      </div>
    </div>
  </div>
</template>
