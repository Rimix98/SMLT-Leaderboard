<script setup lang="ts">
import { ref, watch, nextTick, computed } from 'vue'
import { store } from '../store'
import { addPlayerToRoleApi } from '../api/staff'
import { makeOverlayClose } from '../utils/modal'
import { Plus } from '@lucide/vue'

const props = withDefaults(defineProps<{ visible: boolean; roleIndex?: number }>(), { roleIndex: -1 })
const emit = defineEmits<{ close: [] }>()

const nickname = ref('')
const discord = ref('')
const closeOverlay = makeOverlayClose(() => emit('close'))

const roleName = computed(() => store.staffRoles[props.roleIndex]?.name || '')

watch(() => props.visible, (v) => {
  if (!v) return
  nickname.value = ''
  discord.value = ''
  nextTick(() => { const f = document.getElementById('apNickname'); if (f) f.focus() })
})

async function submit() {
  const nick = nickname.value.trim()
  if (!nick) return
  const disc = discord.value.trim()
  const ok = await addPlayerToRoleApi(props.roleIndex, nick, disc)
  if (ok) emit('close')
}
</script>

<template>
  <div class="modal-overlay" :class="{ active: visible }" @mousedown="closeOverlay.onMousedown" @mouseup="closeOverlay.onMouseup">
    <div class="modal" @mousedown.stop @mouseup.stop>
      <div class="modal-header">
        <div class="modal-title"><Plus :size="16" /> Добавить игрока в «{{ roleName }}»</div>
        <button class="modal-close" @click="emit('close')">✕</button>
      </div>
      <div class="modal-body">
        <div class="form-group">
          <label>Ник игрока:</label>
          <input type="text" id="apNickname" class="form-input" placeholder="Nickname" v-model="nickname" @keyup.enter="submit">
        </div>
        <div class="form-group">
          <label>Discord (необязательно):</label>
          <input type="text" class="form-input" placeholder="Username#0000 or username" v-model="discord" @keyup.enter="submit">
        </div>
        <div class="modal-actions-row-spaced">
          <button class="btn btn-secondary" @click="emit('close')">Отмена</button>
          <button class="btn btn-primary" @click="submit">Добавить игрока</button>
        </div>
      </div>
    </div>
  </div>
</template>
