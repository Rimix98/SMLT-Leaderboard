<script setup lang="ts">
import { ExternalLink } from '@lucide/vue'

const MAP_URL = 'https://smltsetup.duckdns.org'
</script>

<template>
  <section class="smp-section smp-map-section" id="map">
    <div class="smp-container smp-map-layout">
      <div class="smp-map-copy smp-reveal">
        <p class="smp-eyebrow">Весь мир — как на ладони</p>
        <h2>Интерактивная карта мира</h2>
        <p>Исследуйте постройки, города и территории игроков прямо в браузере.</p>
        <a class="smp-btn smp-btn-primary" :href="MAP_URL" target="_blank" rel="noopener noreferrer">
          Открыть карту <ExternalLink :size="14" />
        </a>
      </div>
      <div class="smp-map-preview smp-reveal" aria-label="Стилизованный предварительный просмотр карты">
        <div class="smp-map-grid" aria-hidden="true"></div>
        <span class="smp-map-river" aria-hidden="true"></span>
        <span class="smp-map-pin smp-map-pin-one" aria-hidden="true"><i></i><b>Спавн</b></span>
        <span class="smp-map-pin smp-map-pin-two" aria-hidden="true"><i></i><b>Город</b></span>
        <div class="smp-map-coords" aria-hidden="true">X: 128 &nbsp; Z: −340</div>
      </div>
    </div>
  </section>
</template>
