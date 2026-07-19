<script setup lang="ts">
import { Gamepad2, Users, Star } from '@lucide/vue'
</script>

<template>
  <section class="smp-section smp-about" id="about">
    <div class="smp-container">
      <div class="smp-section-heading smp-reveal">
        <p class="smp-eyebrow">Место, куда НЕ хочется вернуться</p>
        <h2>Minecraft </h2>
        <p>Анархия где сам за себя. Умер? Skill issue.</p>
      </div>
      <div class="smp-feature-grid">
        <article class="smp-feature-card smp-reveal">
          <span class="smp-feature-number">01</span>
          <div class="smp-feature-icon"><Gamepad2 :size="22" /></div>
          <h3>Геймплей</h3>
          <p>Строишь 30 минут свой дом, тебя рейдят и ты умираешь.</p>
        </article>
        <article class="smp-feature-card smp-reveal">
          <span class="smp-feature-number">02</span>
          <div class="smp-feature-icon"><Users :size="22" /></div>
          <h3>Сообщество</h3>
          <p>Игроки ещё более менее, а вот админы нет.</p>
        </article>
        <article class="smp-feature-card smp-reveal">
          <span class="smp-feature-number">03</span>
          <div class="smp-feature-icon"><Star :size="22" /></div>
          <h3>События</h3>
          <p>Какие то ивенты тут будут, но когда? Никто не знает.</p>
        </article>
      </div>
    </div>
  </section>
</template>
