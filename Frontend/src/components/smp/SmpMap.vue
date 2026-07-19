<script setup lang="ts">
import { ExternalLink } from '@lucide/vue'

const MAP_URL = 'https://map.example.com'
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

<style scoped>
.smp-section { padding: 128px 0; }

.smp-eyebrow {
  display: flex;
  align-items: center;
  gap: 9px;
  margin: 0 0 22px;
  color: #5cdb75;
  font-size: .76rem;
  font-weight: 800;
  letter-spacing: .16em;
  text-transform: uppercase;
}

.smp-map-section {
  position: relative;
  overflow: hidden;
  background: var(--color-surface);
}

.smp-map-section::before {
  position: absolute;
  top: -180px;
  right: -140px;
  width: 500px;
  height: 500px;
  content: "";
  background: rgba(92,219,117,.07);
  border-radius: 50%;
  filter: blur(80px);
}

.smp-map-layout {
  display: grid;
  grid-template-columns: .8fr 1.2fr;
  align-items: center;
  gap: 80px;
}

.smp-map-copy .smp-btn { margin-top: 20px; }

.smp-map-copy h2 {
  margin: 0 0 22px;
  font-size: clamp(2.35rem, 5vw, 4.25rem);
  line-height: 1.04;
  letter-spacing: -.05em;
}

.smp-map-copy > p { color: var(--color-text-secondary); font-size: 1.08rem; }

.smp-map-preview {
  position: relative;
  height: 490px;
  overflow: hidden;
  border: 1px solid rgba(255,255,255,.13);
  border-radius: 28px;
  background-color: #233b2a;
  background-image: radial-gradient(circle at 68% 24%, #3c6241 0 7%, transparent 8%), radial-gradient(circle at 30% 75%, #31563a 0 11%, transparent 12%), linear-gradient(135deg, #294832 0 35%, #1d3425 35% 55%, #315138 55%);
  box-shadow: 0 24px 70px rgba(0,0,0,.34);
  transform: perspective(1000px) rotateY(-3deg) rotateX(2deg);
}

.smp-map-grid {
  position: absolute;
  inset: 0;
  opacity: .18;
  background-image: linear-gradient(rgba(255,255,255,.18) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,.18) 1px, transparent 1px);
  background-size: 30px 30px;
  transform: rotate(10deg) scale(1.3);
}

.smp-map-river {
  position: absolute;
  top: -10%;
  left: 33%;
  width: 76px;
  height: 125%;
  background: rgba(67,136,153,.55);
  border: 4px solid rgba(118,181,190,.12);
  border-radius: 50%;
  transform: rotate(23deg);
}

.smp-map-pin {
  position: absolute;
  display: flex;
  align-items: center;
  gap: 7px;
  color: #fff;
  font-size: .7rem;
}

.smp-map-pin i {
  width: 13px;
  height: 13px;
  background: #5cdb75;
  border: 3px solid #fff;
  border-radius: 50%;
  box-shadow: 0 0 0 7px rgba(92,219,117,.16);
}

.smp-map-pin b {
  padding: 4px 8px;
  background: rgba(8,12,9,.72);
  border-radius: 5px;
}

.smp-map-pin-one { top: 37%; left: 56%; }
.smp-map-pin-two { right: 16%; bottom: 23%; }

.smp-map-coords {
  position: absolute;
  right: 18px;
  bottom: 16px;
  padding: 6px 9px;
  color: rgba(255,255,255,.7);
  background: rgba(8,12,9,.65);
  border-radius: 6px;
  font-family: ui-monospace, monospace;
  font-size: .68rem;
}

@media (max-width: 980px) {
  .smp-map-layout { grid-template-columns: 1fr; gap: 55px; }
  .smp-map-copy { max-width: 620px; }
}

@media (max-width: 680px) {
  .smp-section { padding: 90px 0; }
  .smp-map-preview { height: 380px; transform: none; }
}
</style>
