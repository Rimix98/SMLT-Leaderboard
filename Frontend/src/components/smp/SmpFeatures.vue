<script setup lang="ts">
import { Gamepad2, Users, Star } from '@lucide/vue'
</script>

<template>
  <section class="smp-section smp-about" id="about">
    <div class="smp-container">
      <div class="smp-section-heading smp-reveal">
        <p class="smp-eyebrow">Место, куда хочется вернуться</p>
        <h2>Minecraft без лишнего шума</h2>
        <p>Наш сервер создан для спокойной и интересной игры. Здесь можно строить города, исследовать мир, торговать с другими игроками и участвовать в событиях.</p>
      </div>
      <div class="smp-feature-grid">
        <article class="smp-feature-card smp-reveal">
          <span class="smp-feature-number">01</span>
          <div class="smp-feature-icon"><Gamepad2 :size="22" /></div>
          <h3>Выживание</h3>
          <p>Классическое выживание с дополнительными возможностями.</p>
        </article>
        <article class="smp-feature-card smp-reveal">
          <span class="smp-feature-number">02</span>
          <div class="smp-feature-icon"><Users :size="22" /></div>
          <h3>Сообщество</h3>
          <p>Дружные игроки и активная администрация.</p>
        </article>
        <article class="smp-feature-card smp-reveal">
          <span class="smp-feature-number">03</span>
          <div class="smp-feature-icon"><Star :size="22" /></div>
          <h3>События</h3>
          <p>Конкурсы, турниры и сезонные мероприятия.</p>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.smp-section { padding: 128px 0; }

.smp-section-heading {
  max-width: 800px;
  margin-bottom: 55px;
}

.smp-section-heading h2 {
  margin: 0 0 22px;
  font-size: clamp(2.35rem, 5vw, 4.25rem);
  line-height: 1.04;
  letter-spacing: -.05em;
  text-wrap: balance;
}

.smp-section-heading > p:last-child {
  color: var(--color-text-secondary);
  font-size: 1.08rem;
}

.smp-feature-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
}

.smp-feature-card {
  position: relative;
  min-height: 300px;
  padding: 32px;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: 18px;
  background: linear-gradient(145deg, var(--color-surface-3), var(--color-surface-2));
  transition: transform .3s ease, border-color .3s ease, box-shadow .3s ease;
}

.smp-feature-card:hover {
  border-color: rgba(92,219,117,.35);
  box-shadow: 0 18px 50px rgba(0,0,0,.25);
  transform: translateY(-7px);
}

.smp-feature-number {
  position: absolute;
  top: 22px;
  right: 24px;
  color: rgba(255,255,255,.15);
  font-family: ui-monospace, monospace;
  font-size: .75rem;
}

.smp-feature-icon {
  display: grid;
  width: 53px;
  height: 53px;
  margin: 55px 0 30px;
  place-items: center;
  color: #5cdb75;
  border: 1px solid rgba(92,219,117,.25);
  border-radius: 13px;
  background: rgba(92,219,117,.08);
}

.smp-feature-card h3 { margin: 0 0 10px; font-size: 1.35rem; }
.smp-feature-card p { margin: 0; color: var(--color-text-secondary); }

@media (max-width: 680px) {
  .smp-section { padding: 90px 0; }
  .smp-section-heading { margin-bottom: 38px; }
  .smp-section-heading h2 { font-size: 2.55rem; }
  .smp-feature-grid { grid-template-columns: 1fr; }
  .smp-feature-card { min-height: 240px; }
  .smp-feature-icon { margin-top: 35px; }
}
</style>
