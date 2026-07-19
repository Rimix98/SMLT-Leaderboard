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
