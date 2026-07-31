<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import AppShell from './AppShell.vue'
import { Trophy, ClipboardList, Lock } from '@lucide/vue'

const { t } = useI18n()
</script>

<template>
  <AppShell>
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
          <h2 class="hero-title">{{ t('home.heroTitle') }}</h2>
          <p class="hero-description">
            {{ t('home.heroIntro1') }}
          </p>
          <p class="hero-description">
            {{ t('home.heroIntro2') }}
            {{ t('home.heroBody', { highlight: t('home.highlight') }) }}
          </p>
        </div>
      </section>

      <section class="nav-buttons">
        <router-link to="/leaderboard" class="nav-card">
          <span class="nav-card-icon"><Trophy :size="36" /></span>
          <span class="nav-card-title">{{ t('home.leaderboardCardTitle') }}</span>
          <span class="nav-card-desc">{{ t('home.leaderboardCardDesc') }}</span>
        </router-link>

      </section>

      <section class="info-section">
        <div class="info-card">
          <h3 class="info-title"><ClipboardList :size="16" /> {{ t('common.info', 'Информация') }}</h3>
          <div class="info-content">
            <div class="info-item">
              <span class="info-label">{{ t('home.discordServerLabel', 'Discord сервер') }}</span>
              <a href="https://discord.gg/VK56W7ZzdA" target="_blank" class="info-value" style="color: var(--color-secondary);">discord.gg/VK56W7ZzdA</a>
            </div>
            <div class="info-item">
              <span class="info-label">{{ t('home.adminLabel') }}</span>
              <div class="info-value">
                Discord: <span class="highlight">@.samoletik</span><br>
                Telegram: <span class="highlight">@samoletik</span>
              </div>
            </div>
            <div class="info-item security">
              <span class="info-label"><Lock :size="12" /> {{ t('home.securityLabel', 'Security & backend') }}</span>
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
          <p style="color:var(--color-text-muted);font-size:var(--font-size-lg);font-weight:600;letter-spacing:0.05em">{{ t('home.adPlaceholder') }}</p>
        </div>
      </section>
    </div>
  </main>
</template>
