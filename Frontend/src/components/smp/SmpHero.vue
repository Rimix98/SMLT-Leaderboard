<script setup lang="ts">
import { Copy, ExternalLink, ChevronDown } from '@lucide/vue'

defineProps<{
  serverIp: string
}>()

const emit = defineEmits<{
  copyIp: []
}>()

const MAP_URL = 'https://smltsetup.duckdns.org'
const SERVER_DOMAIN = 'smltsetup.duckdns.org'

function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' })
}
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
        <a class="smp-btn smp-btn-primary" href="#start" @click.prevent="scrollTo('start')">Начать играть</a>
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
      <p class="smp-hero-alt">или <code>{{ SERVER_DOMAIN }}</code></p>
    </div>
    <button class="smp-scroll-hint" type="button" @click="scrollTo('status')" aria-label="Прокрутить к статусу сервера">
      <span>Листайте вниз</span>
      <ChevronDown :size="18" />
    </button>
  </section>
</template>
