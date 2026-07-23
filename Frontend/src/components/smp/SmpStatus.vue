<script setup lang="ts">
import type { SMPStatus } from '../../types'

defineProps<{
  status: SMPStatus | null
  loading: boolean
}>()

const emit = defineEmits<{
  copyIp: []
}>()
</script>

<template>
  <section class="smp-status-section" id="status">
    <div class="smp-container">
      <div v-if="loading" class="smp-status-card smp-reveal">
        <div class="smp-status-main">
          <div class="smp-server-cube" aria-hidden="true"><span></span></div>
          <div>
            <p class="smp-status-label">Статус сервера</p>
            <h2 class="smp-status-title"><span class="smp-status-indicator loading"></span> Загрузка...</h2>
          </div>
        </div>
      </div>
      <div v-else-if="status" class="smp-status-card smp-reveal" :class="{ 'is-offline': !status.online }">
        <div class="smp-status-main">
          <div class="smp-server-cube" aria-hidden="true"><span></span></div>
          <div>
            <p class="smp-status-label">Статус сервера</p>
            <h2 class="smp-status-title">
              <span class="smp-status-indicator" :class="{ online: status.online, offline: !status.online }"></span>
              {{ status.online ? '🟢 Работает' : '🔴 Выключен' }}
            </h2>
          </div>
        </div>
        <div class="smp-status-stats">
          <div><dt>Игроки</dt><dd><strong>{{ status.playersOnline }}</strong> / {{ status.playersMax }}</dd></div>
          <div><dt>Версия</dt><dd>{{ status.version }}</dd></div>
          <div><dt>IP-адрес</dt><dd><code>{{ status.serverIp }}</code></dd></div>
        </div>
        <p class="smp-status-refresh">Обновляется автоматически</p>
        <button class="smp-btn smp-btn-primary" type="button" @click="emit('copyIp')">Скопировать IP</button>
      </div>
    </div>
  </section>
</template>
