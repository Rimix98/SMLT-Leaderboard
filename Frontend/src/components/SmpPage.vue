<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import AppShell from './AppShell.vue'
import SmpHero from './smp/SmpHero.vue'
import SmpStatus from './smp/SmpStatus.vue'
import SmpFeatures from './smp/SmpFeatures.vue'
import SmpMap from './smp/SmpMap.vue'
import SmpRules from './smp/SmpRules.vue'
import SmpHowTo from './smp/SmpHowTo.vue'
import SmpFooter from './smp/SmpFooter.vue'
import { fetchSMPStatus } from '../api/smp'
import type { SMPStatus } from '../types'

const SERVER_IP = '94.154.11.166'

const serverStatus = ref<SMPStatus | null>(null)
const loading = ref(true)

const toastVisible = ref(false)
const toastMessage = ref('')
let toastTimer: ReturnType<typeof setTimeout> | null = null

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

onMounted(async () => {
  try {
    serverStatus.value = await fetchSMPStatus()
  } catch {
    serverStatus.value = {
      online: false,
      playersMax: 0,
      playersOnline: 0,
      version: '1.21',
      serverIp: SERVER_IP,
      fetchedAt: new Date().toISOString(),
    }
  } finally {
    loading.value = false
  }

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
})
</script>

<template>
  <AppShell>
    <template #brand>
      <div class="smp-brand">
        <span class="smp-brand-mark">S</span>
        <div>
          <div class="smp-brand-title">SMLT <strong>SMP</strong></div>
          <div class="smp-brand-sub">Minecraft-сервер</div>
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

  <SmpFooter :server-ip="SERVER_IP" @copy-ip="copyIp" />

  <Teleport to="body">
    <div class="smp-toast" :class="{ show: toastVisible }" role="status" aria-live="polite">
      <span>&check;</span> {{ toastMessage }}
    </div>
  </Teleport>
</template>

<style scoped>
.smp-main {
  color: var(--color-text-primary);
  background: var(--color-primary);
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  line-height: 1.6;
  overflow-x: hidden;
}

.smp-main :deep(a) { color: inherit; text-decoration: none; }
.smp-main :deep(button) { color: inherit; font: inherit; }

.smp-container {
  width: min(calc(100% - 40px), 1180px);
  margin-inline: auto;
}

/* Бренд */
.smp-brand {
  display: flex;
  align-items: center;
  gap: 11px;
}

.smp-brand-mark {
  display: grid;
  width: 36px;
  height: 36px;
  place-items: center;
  color: #071008;
  background: #5cdb75;
  border-radius: 8px 8px 11px 8px;
  font-weight: 950;
  font-size: 1rem;
  box-shadow: 7px 7px 0 rgba(92,219,117,.13);
}

.smp-brand-title {
  font-size: 1.05rem;
  font-weight: 850;
  letter-spacing: .03em;
}

.smp-brand-title strong { color: #5cdb75; }

.smp-brand-sub {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
  margin-top: -2px;
}

/* Кнопки */
.smp-btn {
  display: inline-flex;
  min-height: 52px;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 0 24px;
  border: 1px solid transparent;
  border-radius: 12px;
  cursor: pointer;
  font-weight: 750;
  font-size: var(--font-size-md);
  letter-spacing: -0.01em;
  transition: transform .25s ease, background-color .25s ease, border-color .25s ease, box-shadow .25s ease;
}

.smp-btn:hover { transform: translateY(-3px); }

.smp-btn-primary {
  color: #071008;
  background: #5cdb75;
  box-shadow: 0 12px 30px rgba(92, 219, 117, .16);
}

.smp-btn-primary:hover {
  background: #78ed8d;
  box-shadow: 0 16px 36px rgba(92, 219, 117, .25);
}

.smp-btn-ghost {
  color: var(--color-text-primary);
  border-color: rgba(255,255,255,.25);
  background: rgba(9,12,10,.34);
  backdrop-filter: blur(12px);
}

.smp-btn-ghost:hover {
  border-color: rgba(255,255,255,.5);
  background: rgba(255,255,255,.08);
}

/* Анимации */
.smp-reveal {
  opacity: 0;
  transform: translateY(28px);
  transition: opacity .7s cubic-bezier(.2,.65,.25,1), transform .7s cubic-bezier(.2,.65,.25,1);
}

.smp-reveal.visible { opacity: 1; transform: translateY(0); }

/* Toast */
.smp-toast {
  position: fixed;
  z-index: 200;
  right: 24px;
  bottom: 24px;
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 14px 18px;
  color: #071008;
  background: #5cdb75;
  border-radius: 11px;
  box-shadow: 0 15px 50px rgba(0,0,0,.4);
  font-weight: 800;
  opacity: 0;
  pointer-events: none;
  transform: translateY(20px);
  transition: opacity .25s ease, transform .25s ease;
}

.smp-toast.show { opacity: 1; transform: translateY(0); }

.smp-toast span {
  display: grid;
  width: 20px;
  height: 20px;
  place-items: center;
  color: #5cdb75;
  background: #071008;
  border-radius: 50%;
  font-size: .72rem;
}
</style>
