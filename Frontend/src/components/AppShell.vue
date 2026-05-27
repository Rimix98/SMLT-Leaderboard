<script setup>
import { ref, onMounted, provide, watch, nextTick } from 'vue'
import { store, toggleTheme } from '../store'
import { initHostStatus, initCaptcha, verifyHost, logoutHost } from '../api/auth'
import { refreshCsrfToken } from '../api/utils'
import { useRouter } from '../router'

const { navigate } = useRouter()

const props = defineProps({ page: { type: String, default: '' } })

const navRef = ref(null)
const navIndicatorStyle = ref({ left: '0px', width: '0px' })

function updateNavIndicator() {
  if (!navRef.value) return
  const activeLink = navRef.value.querySelector('.nav-link.active')
  if (!activeLink) return
  const navRect = navRef.value.getBoundingClientRect()
  const linkRect = activeLink.getBoundingClientRect()
  navIndicatorStyle.value = {
    left: `${linkRect.left - navRect.left}px`,
    width: `${linkRect.width}px`,
  }
}

watch(() => props.page, () => {
  nextTick(updateNavIndicator)
})

// Modal states
const hostModalOpen = ref(false)
const hostPassword = ref('')
const captchaValue = ref('')
const hostError = ref('')
const infoModalOpen = ref(false)

watch(() => store.isHost, (val) => {
  document.body.classList.toggle('host-active', val)
}, { immediate: true })

onMounted(async () => {
  await refreshCsrfToken()
  await initHostStatus()
  nextTick(updateNavIndicator)
})

function openHostModal() {
  hostModalOpen.value = true
  hostPassword.value = ''
  captchaValue.value = ''
  hostError.value = ''
  setTimeout(() => initCaptcha(), 50)
}

function closeHostModal() {
  hostModalOpen.value = false
}

async function doVerify() {
  hostError.value = ''
  const res = await verifyHost(hostPassword.value)
  if (store.isHost) hostModalOpen.value = false
}

function openInfoModal() {
  infoModalOpen.value = true
}

function closeInfoModal() {
  infoModalOpen.value = false
}

// Provide for child components
provide('openHostModal', openHostModal)
provide('openInfoModal', openInfoModal)
provide('closeInfoModal', closeInfoModal)
</script>

<template>
  <button class="theme-toggle" title="Сменить тему" @click="toggleTheme">
    <span class="theme-icon">{{ store.theme === 'dark' ? '🌙' : '☀️' }}</span>
  </button>

  <button
    class="host-btn host-btn-left"
    :class="{ 'is-host': store.isHost }"
    title="Войти как хост"
    @click="store.isHost ? logoutHost() : openHostModal()"
  >
    <span>{{ store.isHost ? '👑 Хост' : 'Хост' }}</span>
  </button>

  <header class="app-header">
    <div class="header-content">
      <div class="header-brand">
        <slot name="brand" />
      </div>
      <nav class="header-nav" ref="navRef">
        <div class="nav-indicator" :style="navIndicatorStyle"></div>
        <a href="#" class="nav-link" :class="{ active: props.page === 'home' }" @click.prevent="navigate('home')">Главная</a>
        <a href="#" class="nav-link" :class="{ active: props.page === 'leaderboard' }" @click.prevent="navigate('leaderboard')">Лидерборд</a>
        <a href="#" class="nav-link" :class="{ active: props.page === 'projects' }" @click.prevent="navigate('projects')">Проекты</a>
        <a href="#" class="nav-link" :class="{ active: props.page === 'staff' }" @click.prevent="navigate('staff')">Стафф</a>
      </nav>
      <div class="header-actions">
        <slot name="actions" />
      </div>
    </div>
  </header>

  <Teleport to="body">
    <div class="modal-overlay" :class="{ active: hostModalOpen }" @click.self="closeHostModal">
      <div class="modal" @click.stop>
        <div class="modal-header">
          <div class="modal-title">🔐 Вход хоста</div>
          <button class="modal-close" @click="closeHostModal">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label for="hostPassword">Введите пароль:</label>
            <input type="password" class="form-input" placeholder="Пароль" v-model="hostPassword" @keyup.enter="doVerify">
          </div>
          <div class="form-group">
            <div class="captcha-row">
              <img id="captcha-img" class="captcha-image" alt="Капча" src="">
              <button type="button" class="btn btn-secondary" title="Обновить капчу" @click="initCaptcha">🔄</button>
            </div>
            <input type="text" id="captchaInput" class="form-input" placeholder="Код с картинки" maxlength="6" autocomplete="off" v-model="captchaValue">
          </div>
          <button class="btn btn-primary" @click="doVerify">Войти</button>
          <div v-if="hostError" class="error-message">{{ hostError }}</div>
        </div>
      </div>
    </div>

    <div class="modal-overlay" :class="{ active: infoModalOpen }" @click.self="closeInfoModal">
      <div class="modal" style="max-width: 480px;" @click.stop>
        <div class="modal-header">
          <div class="modal-title">📋 Информация</div>
          <button class="modal-close" @click="closeInfoModal">✕</button>
        </div>
        <div class="modal-body">
          <div style="display: flex; flex-direction: column; gap: var(--spacing-md);">
            <div style="padding: var(--spacing-md); background: var(--color-surface-2); border-radius: var(--border-radius-md);">
              <div style="color: var(--color-text-muted); font-size: var(--font-size-sm); margin-bottom: var(--spacing-xs);">Discord Server</div>
              <a href="https://discord.gg/VK56W7ZzdA" target="_blank" style="color: var(--color-secondary); font-weight: 600;">discord.gg/VK56W7ZzdA</a>
            </div>
            <div style="padding: var(--spacing-md); background: var(--color-surface-2); border-radius: var(--border-radius-md);">
              <div style="color: var(--color-text-muted); font-size: var(--font-size-sm); margin-bottom: var(--spacing-xs);">Admin</div>
              <div style="font-weight: 500;">Discord: <span style="color: var(--color-secondary);">@.samoletik</span></div>
              <div style="font-weight: 500; margin-top: var(--spacing-xs);">Telegram: <span style="color: var(--color-secondary);">@samoltik</span></div>
            </div>
            <div style="padding: var(--spacing-md); background: var(--color-surface-2); border-radius: var(--border-radius-md); border-left: 3px solid var(--color-success);">
              <div style="color: var(--color-text-muted); font-size: var(--font-size-sm); margin-bottom: var(--spacing-xs);">🔒 За безопасность и бекенд отвечал</div>
              <div style="font-weight: 500;">Discord: <span style="color: var(--color-secondary);">@rimix.98</span></div>
              <div style="font-weight: 500; margin-top: var(--spacing-xs);">Telegram: <span style="color: var(--color-secondary);">@Rimix980</span></div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
  <div id="toastContainer"></div>
</template>

<style>
@import '../../styles.css';
</style>
