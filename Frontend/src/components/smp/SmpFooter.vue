<script setup lang="ts">
import { ExternalLink } from '@lucide/vue'

defineProps<{
  serverIp: string
}>()

const emit = defineEmits<{
  copyIp: []
}>()

const MAP_URL = 'https://map.example.com'
const DISCORD_URL = 'https://discord.gg/VK56W7ZzdA'
</script>

<template>
  <footer class="smp-footer">
    <div class="smp-container smp-footer-top">
      <div class="smp-footer-brand">
        <div class="smp-brand-inline">
          <span class="smp-brand-mark">S</span>
          <span>SMLT <strong>SMP</strong></span>
        </div>
        <p>Уютный мир для больших приключений.</p>
      </div>
      <div class="smp-footer-column">
        <h4>Сервер</h4>
        <button class="smp-footer-link" type="button" @click="emit('copyIp')">{{ serverIp }}</button>
        <a class="smp-footer-link" :href="MAP_URL" target="_blank" rel="noopener noreferrer">Карта мира</a>
      </div>
      <div class="smp-footer-column">
        <h4>Сообщество</h4>
        <a class="smp-footer-link" :href="DISCORD_URL" target="_blank" rel="noopener noreferrer">Discord <ExternalLink :size="12" /></a>
        <a class="smp-footer-link" href="#rules">Правила</a>
      </div>
    </div>
    <div class="smp-container smp-footer-bottom">
      <p>&copy; {{ new Date().getFullYear() }} SMLT SMP</p>
      <p>Сайт не является официальным продуктом Mojang или Microsoft.</p>
    </div>
  </footer>
</template>

<style scoped>
.smp-brand-mark {
  display: grid;
  width: 36px;
  height: 36px;
  place-items: center;
  color: #071008;
  background: #5cdb75;
  border-radius: 8px 8px 11px 8px;
  font-weight: 950;
  font-size: 1rem;
  box-shadow: 7px 7px 0 rgba(92,219,117,.13);
}

.smp-brand-inline {
  display: inline-flex;
  align-items: center;
  gap: 11px;
  font-size: 1.05rem;
  font-weight: 850;
  letter-spacing: .03em;
}

.smp-brand-inline strong { color: #5cdb75; }

.smp-footer {
  padding-top: 70px;
  background: var(--color-surface);
  border-top: 1px solid var(--color-border);
}

.smp-footer-top {
  display: grid;
  grid-template-columns: 1.7fr 1fr 1fr;
  gap: 50px;
  padding-bottom: 60px;
}

.smp-footer-brand p { color: var(--color-text-secondary); margin-top: 8px; }

.smp-footer-column {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
}

.smp-footer-column h4 {
  margin: 0 0 7px;
  color: var(--color-text-muted);
  font-size: .7rem;
  letter-spacing: .12em;
  text-transform: uppercase;
}

.smp-footer-link {
  padding: 0;
  color: var(--color-text-muted);
  border: 0;
  background: none;
  cursor: pointer;
  font-size: .9rem;
  transition: color .2s ease;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.smp-footer-link:hover { color: #5cdb75; }

.smp-footer-bottom {
  display: flex;
  justify-content: space-between;
  gap: 20px;
  padding: 22px 0;
  color: var(--color-text-muted);
  border-top: 1px solid var(--color-border);
  font-size: .75rem;
}

.smp-footer-bottom p { margin: 0; }

@media (max-width: 680px) {
  .smp-footer-top { grid-template-columns: 1fr 1fr; }
  .smp-footer-brand { grid-column: 1 / -1; }
  .smp-footer-bottom { flex-direction: column; }
}
</style>
