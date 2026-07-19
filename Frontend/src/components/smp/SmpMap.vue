<script setup lang="ts">
import { ExternalLink } from '@lucide/vue'

const MAP_URL = 'https://smltsetup.duckdns.org'
</script>

<template>
  <section class="smp-section smp-map-section" id="map">
    <div class="smp-container smp-map-center smp-reveal">
      <p class="smp-eyebrow">Весь мир — как на ладони</p>
      <h2>Интерактивная карта мира</h2>
      <p>Исследуйте постройки, города и территории игроков прямо в браузере.</p>
      <a class="smp-btn smp-btn-primary" :href="MAP_URL" target="_blank" rel="noopener noreferrer">
        Открыть карту <ExternalLink :size="14" />
      </a>
    </div>
  </section>
</template>
