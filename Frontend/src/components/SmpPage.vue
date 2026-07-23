<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import AppShell from './AppShell.vue'
import SmpHero from './smp/SmpHero.vue'
import SmpStatus from './smp/SmpStatus.vue'
import SmpFeatures from './smp/SmpFeatures.vue'
import SmpMap from './smp/SmpMap.vue'
import SmpRules from './smp/SmpRules.vue'
import SmpHowTo from './smp/SmpHowTo.vue'
import { fetchSMPStatus } from '../api/smp'
import type { SMPStatus } from '../types'
import { Globe } from '@lucide/vue'

const SERVER_IP = '94.154.11.166'

const serverStatus = ref<SMPStatus | null>(null)
const loading = ref(true)

const toastVisible = ref(false)
const toastMessage = ref('')
let toastTimer: ReturnType<typeof setTimeout> | null = null
let refreshTimer: ReturnType<typeof setInterval> | null = null

function showToast(message: string) {
  toastMessage.value = message
  toastVisible.value = true
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastVisible.value = false }, 2200)
}

async function copyIp() {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(SERVER_IP)
    } else {
      const el = document.createElement('textarea')
      el.value = SERVER_IP
      el.setAttribute('readonly', '')
      el.style.position = 'fixed'
      el.style.opacity = '0'
      document.body.appendChild(el)
      el.select()
      document.execCommand('copy')
      el.remove()
    }
    showToast('IP скопирован')
  } catch {
    showToast('Не удалось скопировать IP')
  }
}

let revealObserver: IntersectionObserver | null = null

async function refreshStatus() {
  try {
    serverStatus.value = await fetchSMPStatus()
  } catch {
    serverStatus.value = {
      online: false,
      playersOnline: 0,
      playersMax: 0,
      version: '—',
      serverIp: SERVER_IP,
      fetchedAt: new Date().toISOString(),
    }
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await refreshStatus()
  refreshTimer = setInterval(refreshStatus, 45_000)

  const prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const items = document.querySelectorAll('.smp-reveal')
  if (prefersReduced || !('IntersectionObserver' in window)) {
    items.forEach(el => el.classList.add('visible'))
  } else {
    revealObserver = new IntersectionObserver((entries, obs) => {
      entries.forEach(entry => {
        if (entry.isIntersecting) {
          entry.target.classList.add('visible')
          obs.unobserve(entry.target)
        }
      })
    }, { threshold: 0.12 })
    items.forEach(el => revealObserver!.observe(el))
  }
})

onUnmounted(() => {
  revealObserver?.disconnect()
  if (toastTimer) clearTimeout(toastTimer)
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<template>
  <AppShell>
    <template #brand>
      <div class="smp-brand">
        <Globe :size="20" class="smp-brand-icon" />
        <div>
          <div class="smp-brand-title">SMLT SMP</div>
          <div class="smp-brand-sub">Анархический сервер SMLT</div>
        </div>
      </div>
    </template>
  </AppShell>

  <main class="smp-main">
    <SmpHero :server-ip="SERVER_IP" @copy-ip="copyIp" />
    <SmpStatus :status="serverStatus" :loading="loading" @copy-ip="copyIp" />
    <SmpFeatures />
    <SmpMap />
    <SmpRules />
    <SmpHowTo :server-ip="SERVER_IP" @copy-ip="copyIp" />
  </main>

  <Teleport to="body">
    <div class="smp-toast" :class="{ show: toastVisible }" role="status" aria-live="polite">
      <span>&check;</span> {{ toastMessage }}
    </div>
  </Teleport>
</template>
