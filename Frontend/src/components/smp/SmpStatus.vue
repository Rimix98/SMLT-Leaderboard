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
              <span class="smp-status-indicator"></span>
              {{ status.online ? 'Онлайн' : 'Офлайн' }}
            </h2>
          </div>
        </div>
        <div class="smp-status-stats">
          <div><dt>Игроки</dt><dd><strong>{{ status.playersOnline }}</strong> из {{ status.playersMax }}</dd></div>
          <div><dt>Версия</dt><dd>{{ status.version }}</dd></div>
          <div><dt>IP-адрес</dt><dd><code>{{ status.serverIp }}</code></dd></div>
        </div>
        <button class="smp-btn smp-btn-primary" type="button" @click="emit('copyIp')">Скопировать IP</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.smp-status-section {
  position: relative;
  z-index: 5;
  margin-top: -48px;
}

.smp-status-card {
  display: flex;
  align-items: center;
  gap: 42px;
  padding: 28px 32px;
  border: 1px solid var(--color-border);
  border-radius: 18px;
  background: var(--color-surface-2);
  box-shadow: 0 24px 70px rgba(0, 0, 0, 0.34);
  backdrop-filter: blur(18px);
}

.smp-status-main {
  display: flex;
  min-width: 210px;
  align-items: center;
  gap: 16px;
}

.smp-server-cube {
  position: relative;
  width: 48px;
  height: 48px;
  background: #397d45;
  clip-path: polygon(50% 0,100% 25%,100% 75%,50% 100%,0 75%,0 25%);
}

.smp-server-cube::before {
  position: absolute;
  inset: 0;
  content: "";
  background: linear-gradient(30deg, #24572d 50%, transparent 50%);
}

.smp-server-cube span {
  position: absolute;
  z-index: 1;
  top: 8px;
  right: 9px;
  width: 10px;
  height: 8px;
  background: #5cdb75;
  opacity: .8;
}

.smp-status-label {
  margin: 0 0 3px;
  color: var(--color-text-muted);
  font-size: .72rem;
  letter-spacing: .08em;
  text-transform: uppercase;
}

.smp-status-title {
  display: flex;
  align-items: center;
  gap: 9px;
  margin: 0;
  font-size: 1.25rem;
}

.smp-status-indicator {
  width: 8px;
  height: 8px;
  background: #5cdb75;
  border-radius: 50%;
  box-shadow: 0 0 13px #5cdb75;
}

.smp-status-indicator.loading {
  background: var(--color-text-muted);
  box-shadow: none;
  animation: smp-pulse 2s infinite;
}

.smp-status-card.is-offline .smp-status-indicator {
  background: #ef5c5c;
  box-shadow: 0 0 13px #ef5c5c;
}

.smp-status-stats {
  display: grid;
  flex: 1;
  grid-template-columns: repeat(3, 1fr);
}

.smp-status-stats div {
  padding: 0 30px;
  border-left: 1px solid var(--color-border);
}

.smp-status-stats dt {
  color: var(--color-text-muted);
  font-size: .72rem;
  letter-spacing: .06em;
  text-transform: uppercase;
}

.smp-status-stats dd {
  margin: 3px 0 0;
  font-size: 1rem;
  font-weight: 750;
}

.smp-status-stats strong { color: #5cdb75; }
.smp-status-stats code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }

@keyframes smp-pulse { 50% { box-shadow: 0 0 0 10px rgba(92,219,117,0); } }

@media (max-width: 680px) {
  .smp-status-section { margin-top: -25px; }
  .smp-status-card { display: block; padding: 25px 22px; }
  .smp-status-main { margin-bottom: 25px; }
  .smp-status-stats {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 18px 0;
    padding-top: 22px;
    border-top: 1px solid var(--color-border);
  }
  .smp-status-stats div { padding: 0; border: 0; }
  .smp-status-stats div:last-child { grid-column: 1 / -1; }
  .smp-status-card > .smp-btn { width: 100%; margin-top: 24px; }
}
</style>
