<script setup>
import { computed, watch } from 'vue'
import { useRouter } from './router'
import { initTheme } from './store'
import { refreshCsrfToken } from './api/utils'
import HomePage from './components/HomePage.vue'
import LeaderboardPage from './components/LeaderboardPage.vue'
import ProjectsPage from './components/ProjectsPage.vue'
import StaffPage from './components/StaffPage.vue'

initTheme()
refreshCsrfToken()

const { currentPage } = useRouter()

const pageComponent = computed(() => {
  switch (currentPage.value) {
    case 'leaderboard': return LeaderboardPage
    case 'projects': return ProjectsPage
    case 'staff': return StaffPage
    default: return HomePage
  }
})

watch(currentPage, (page) => {
  const titles = { home: 'SMLT - Главная', leaderboard: 'Лидерборд -- SMLT', projects: 'Проекты SMLT', staff: 'Стафф -- SMLT' }
  document.title = titles[page] || 'SMLT'
  document.body.className = page === 'home' ? 'home-page' : ''
}, { immediate: true })
</script>

<template>
  <component :is="pageComponent" />
</template>
