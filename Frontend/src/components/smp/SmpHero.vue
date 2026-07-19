<script setup lang="ts">
import { Copy, ExternalLink, ChevronDown } from '@lucide/vue'

defineProps<{
  serverIp: string
}>()

const emit = defineEmits<{
  copyIp: []
}>()

const MAP_URL = 'https://smltsetup.duckdns.org'
</script>

<template>
  <section class="smp-hero">
    <div class="smp-hero-bg" aria-hidden="true"></div>
    <div class="smp-hero-noise" aria-hidden="true"></div>
    <div class="smp-container smp-hero-content smp-reveal">
      <p class="smp-eyebrow"><span class="smp-live-dot"></span> Сервер уже открыт</p>
      <h1 class="smp-hero-title">Добро пожаловать на<br><span>SMLT SMP</span></h1>
      <p class="smp-hero-text">Не очень то и уютный Minecraft-сервер с выживанием, приключениями и не особо дружным сообществом</p>
      <div class="smp-hero-actions">
        <a class="smp-btn smp-btn-primary" href="#start">Начать играть</a>
        <a class="smp-btn smp-btn-ghost" :href="MAP_URL" target="_blank" rel="noopener noreferrer">
          Открыть карту мира <ExternalLink :size="14" />
        </a>
      </div>
      <div class="smp-hero-ip">
        <span class="smp-hero-ip-label">IP сервера</span>
        <code>{{ serverIp }}</code>
        <button class="smp-icon-copy" type="button" aria-label="Скопировать IP-адрес" @click="emit('copyIp')">
          <Copy :size="16" />
        </button>
      </div>
    </div>
    <a class="smp-scroll-hint" href="#status" aria-label="Прокрутить к статусу сервера">
      <span>Листайте вниз</span>
      <ChevronDown :size="18" />
    </a>
  </section>
</template>
