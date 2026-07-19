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
