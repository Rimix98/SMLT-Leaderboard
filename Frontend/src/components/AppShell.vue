<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, nextTick, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { store, setTheme } from '../store'
import { initHostStatus, initCaptcha, verifyHost, logoutHost } from '../api/auth'
import { refreshCsrfToken } from '../api/utils'
import { makeOverlayClose } from '../utils/modal'
import { setLocale, availableLocales, currentLocale } from '../i18n'
import type { LocaleCode } from '../types'
import {
  Palette, Moon, Sun, CloudFog, Crown, Info, Lock, RefreshCw, Languages,
} from '@lucide/vue'

const { t } = useI18n()
const route = useRoute()
const currentPage = computed(() => route.name)

const themeOpen = ref(false)
const langOpen = ref(false)
const themeBtnRef = ref<HTMLElement | null>(null)
const langBtnRef = ref<HTMLElement | null>(null)

function toggleThemeDropdown() {
  themeOpen.value = !themeOpen.value
  langOpen.value = false
}

function toggleLangDropdown() {
  langOpen.value = !langOpen.value
  themeOpen.value = false
}

function onDocumentClick(e: MouseEvent) {
  const target = e.target as Node
  if (themeOpen.value && themeBtnRef.value && !(themeBtnRef.value as HTMLElement).contains(target)) {
    themeOpen.value = false
  }
  if (langOpen.value && langBtnRef.value && !(langBtnRef.value as HTMLElement).contains(target)) {
    langOpen.value = false
  }
}

onMounted(() => document.addEventListener('click', onDocumentClick))
onUnmounted(() => document.removeEventListener('click', onDocumentClick))

const navRef = ref<HTMLElement | null>(null)
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

watch(() => route.name, () => {
  nextTick(updateNavIndicator)
})

const hostModalOpen = ref(false)
const hostPassword = ref('')
const captchaValue = ref('')
const hostError = ref('')
const infoModalOpen = ref(false)

const hostClose = makeOverlayClose(closeHostModal)
const infoClose = makeOverlayClose(closeInfoModal)

watch(() => store.isHost, (val) => {
  document.body.classList.toggle('host-active', val)
}, { immediate: true })

watch(hostModalOpen, (val) => {
  document.body.classList.toggle('modal-open', val)
})

watch(infoModalOpen, (val) => {
  document.body.classList.toggle('modal-open', val)
})

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
  await verifyHost(hostPassword.value)
  if (store.isHost) hostModalOpen.value = false
}

function openInfoModal() {
  infoModalOpen.value = true
}

function closeInfoModal() {
  infoModalOpen.value = false
}

function pickLocale(code: LocaleCode) {
  setLocale(code)
  langOpen.value = false
}

</script>

<template>
  <div class="theme-dropdown" ref="themeBtnRef">
    <button class="theme-toggle" :title="t('theme.toggle')" @click="toggleThemeDropdown"><Palette :size="18" /></button>
    <div class="theme-dropdown-menu" v-if="themeOpen">
      <button class="theme-option" :class="{ active: store.theme === 'dark' }" @click="setTheme('dark'); themeOpen = false">
        <span class="theme-option-icon"><Moon :size="16" /></span> {{ $t('theme.dark') }}
      </button>
      <button class="theme-option" :class="{ active: store.theme === 'light' }" @click="setTheme('light'); themeOpen = false">
        <span class="theme-option-icon"><Sun :size="16" /></span> {{ $t('theme.light') }}
      </button>
      <button class="theme-option" :class="{ active: store.theme === 'gray' }" @click="setTheme('gray'); themeOpen = false">
        <span class="theme-option-icon"><CloudFog :size="16" /></span> {{ $t('theme.gray') }}
      </button>
    </div>
  </div>

  <div class="lang-dropdown" ref="langBtnRef">
    <button class="theme-toggle" :title="t('lang.switchLabel')" @click="toggleLangDropdown">
      <Languages :size="18" />
    </button>
    <div class="theme-dropdown-menu" v-if="langOpen">
      <button
        v-for="loc in availableLocales"
        :key="loc.code"
        class="theme-option"
        :class="{ active: currentLocale === loc.code }"
        @click="pickLocale(loc.code)"
      >
        {{ loc.nativeLabel }}
      </button>
    </div>
  </div>

  <button
    class="host-btn host-btn-left"
    :class="{ 'is-host': store.isHost }"
    :title="t('host.loginTitle')"
    @click="store.isHost ? logoutHost() : openHostModal()"
  >
    <span v-if="store.isHost"><Crown :size="14" /> {{ t('host.label') }}</span>
    <span v-else>{{ t('host.label') }}</span>
  </button>

  <header class="app-header">
    <div class="header-content">
      <div class="header-brand">
        <slot name="brand" />
      </div>
      <nav class="header-nav" ref="navRef">
        <div class="nav-indicator" :style="navIndicatorStyle"></div>
        <router-link to="/" class="nav-link" :class="{ active: currentPage === 'home' }">{{ t('nav.home') }}</router-link>
        <router-link to="/leaderboard" class="nav-link" :class="{ active: currentPage === 'leaderboard' }">{{ t('nav.leaderboard') }}</router-link>

        <router-link to="/smp" class="nav-link" :class="{ active: currentPage === 'smp' }">{{ t('nav.smp') }}</router-link>

      </nav>
      <div class="header-actions">
        <button class="btn btn-secondary btn-lg" @click="openInfoModal"><Info :size="16" /> {{ t('common.info', 'Информация') }}</button>
        <slot name="actions" />
      </div>
    </div>
  </header>

  <Teleport to="body">
    <div class="modal-overlay" :class="{ active: hostModalOpen }" @mousedown="hostClose.onMousedown" @mouseup="hostClose.onMouseup">
      <div class="modal" @mousedown.stop @mouseup.stop>
        <div class="modal-header">
          <div class="modal-title"><Lock :size="16" /> {{ t('host.loginTitle') }}</div>
          <button class="modal-close" @click="closeHostModal">✕</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label for="hostPassword">{{ t('host.passwordLabel') }}</label>
            <input type="password" id="hostPassword" class="form-input" :placeholder="t('host.passwordPlaceholder')" v-model="hostPassword" @keyup.enter="doVerify">
          </div>
          <div class="form-group">
            <div class="captcha-row">
              <img id="captcha-img" class="captcha-image" :alt="t('host.captchaImage')" src="">
              <button type="button" class="btn btn-secondary" :title="t('host.captchaRefresh')" @click="initCaptcha"><RefreshCw :size="16" /></button>
            </div>
            <input type="text" id="captchaInput" class="form-input" :placeholder="t('host.captchaPlaceholder')" maxlength="6" autocomplete="off" v-model="captchaValue">
          </div>
          <div class="modal-actions-row">
            <button class="btn btn-primary btn-full-width" @click="doVerify">{{ t('host.login') }}</button>
          </div>
          <div v-if="hostError" class="error-message">{{ hostError }}</div>
        </div>
      </div>
    </div>

    <div class="modal-overlay" :class="{ active: infoModalOpen }" @mousedown="infoClose.onMousedown" @mouseup="infoClose.onMouseup">
      <div class="modal" style="max-width: 480px;" @mousedown.stop @mouseup.stop>
        <div class="modal-header">
          <div class="modal-title"><Info :size="16" /> {{ t('common.info', 'Информация') }}</div>
          <button class="modal-close" @click="closeInfoModal">✕</button>
        </div>
        <div class="modal-body">
          <div style="display: flex; flex-direction: column; gap: var(--spacing-md);">
            <div style="padding: var(--spacing-md); background: var(--color-surface-2); border-radius: var(--border-radius-md); border: 1px solid var(--color-border);">
              <div style="color: var(--color-text-muted); font-size: var(--font-size-xs); margin-bottom: var(--spacing-xs); font-weight: 500; text-transform: uppercase; letter-spacing: 0.04em;">Discord Server</div>
              <a href="https://discord.gg/VK56W7ZzdA" target="_blank" style="color: var(--color-secondary); font-weight: 600;">discord.gg/VK56W7ZzdA</a>
            </div>
            <div style="padding: var(--spacing-md); background: var(--color-surface-2); border-radius: var(--border-radius-md); border: 1px solid var(--color-border);">
              <div style="color: var(--color-text-muted); font-size: var(--font-size-xs); margin-bottom: var(--spacing-xs); font-weight: 500; text-transform: uppercase; letter-spacing: 0.04em;">{{ t('home.adminLabel') }}</div>
              <div style="font-weight: 500;">Discord: <span style="color: var(--color-secondary);">@.samoletik</span></div>
              <div style="font-weight: 500; margin-top: var(--spacing-xs);">Telegram: <span style="color: var(--color-secondary);">@samoletik</span></div>
            </div>
            <div style="padding: var(--spacing-md); background: var(--color-surface-2); border-radius: var(--border-radius-md); border: 1px solid var(--color-border); border-left: 3px solid var(--color-success);">
              <div style="color: var(--color-text-muted); font-size: var(--font-size-xs); margin-bottom: var(--spacing-xs); font-weight: 500; text-transform: uppercase; letter-spacing: 0.04em;"><Lock :size="12" /> Security &amp; backend</div>
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
