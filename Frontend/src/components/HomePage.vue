<script setup>
import { onMounted } from 'vue'
import { store, initTheme } from '../store'
import { refreshCsrfToken } from '../api/utils'
import AppShell from './AppShell.vue'

onMounted(async () => {
  initTheme()
  await refreshCsrfToken()
})
</script>

<template>
  <AppShell page="home">
    <template #brand>
      <div class="header-title">
        <h1>SMLT</h1>
        <span class="header-subtitle">Discord Community</span>
      </div>
    </template>
  </AppShell>

  <main class="app-main home-page">
    <div class="container">
      <section class="hero-section">
        <div class="hero-content">
          <h2 class="hero-title">Что такое SMLT?</h2>
          <p class="hero-description">
            SMLT - это дискорд сервер, где происходят разные ивенты, например коллабы, турниры,
            прохождения уровней по частям, и даже игры в майнкрафте.
          </p>
          <p class="hero-description">
            Все участники сервера - участники SMLT, независимо от его скилла.
            Идея нашего сервера - <strong style="color: #4a9eff;">удовольствие</strong>.
          </p>
        </div>
      </section>

      <section class="nav-buttons">
        <a href="leaderboard.html" class="nav-card">
          <span class="nav-card-icon">🏆</span>
          <span class="nav-card-title">Лидерборд</span>
          <span class="nav-card-desc">Лучшие слееры SMLT</span>
        </a>
        <a href="projects.html" class="nav-card">
          <span class="nav-card-icon">📁</span>
          <span class="nav-card-title">Проекты SMLT</span>
          <span class="nav-card-desc">Коллабы SMLT</span>
        </a>
        <a href="staff.html" class="nav-card">
          <span class="nav-card-icon">👥</span>
          <span class="nav-card-title">Стафф SMLT</span>
          <span class="nav-card-desc">Состав команды</span>
        </a>
      </section>

      <section class="info-section">
        <div class="info-card">
          <h3 class="info-title">📋 Информация</h3>
          <div class="info-content">
            <div class="info-item">
              <span class="info-label">Discord сервер</span>
              <a href="https://discord.gg/VK56W7ZzdA" target="_blank" class="info-value" style="color: var(--color-secondary);">discord.gg/VK56W7ZzdA</a>
            </div>
            <div class="info-item">
              <span class="info-label">Админ</span>
              <div class="info-value">
                Discord: <span class="highlight">@.samoletik</span><br>
                Telegram: <span class="highlight">@samoltik</span>
              </div>
            </div>
            <div class="info-item security">
              <span class="info-label">🔒 За безопасность и бекенд отвечал</span>
              <div style="font-weight: 500;">
                Discord: <span class="highlight">@rimix.98</span><br>
                Telegram: <span class="highlight">@Rimix980</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="info-section" style="padding-bottom:var(--spacing-2xl)">
        <div class="info-card" style="text-align:center;border:2px dashed var(--color-border)">
          <p style="color:var(--color-text-muted);font-size:var(--font-size-lg);font-weight:600;letter-spacing:0.05em">Здесь могла быть ваша реклама</p>
        </div>
      </section>
    </div>
  </main>
</template>
