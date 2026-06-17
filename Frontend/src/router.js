import { createRouter, createWebHistory } from 'vue-router'
import HomePage from './components/HomePage.vue'
import LeaderboardPage from './components/LeaderboardPage.vue'
import ProjectsPage from './components/ProjectsPage.vue'
import StaffPage from './components/StaffPage.vue'

const routes = [
  { path: '/', name: 'home', component: HomePage, meta: { title: 'SMLT - Главная', bodyClass: 'home-page' } },
  { path: '/leaderboard', name: 'leaderboard', component: LeaderboardPage, meta: { title: 'Лидерборд -- SMLT', bodyClass: '' } },
  { path: '/projects', name: 'projects', component: ProjectsPage, meta: { title: 'Проекты SMLT', bodyClass: '' } },
  { path: '/staff', name: 'staff', component: StaffPage, meta: { title: 'Стафф -- SMLT', bodyClass: '' } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(to, from, savedPosition) {
    if (savedPosition) return savedPosition
    return { top: 0, behavior: 'smooth' }
  },
})

router.afterEach((to) => {
  document.title = to.meta.title || 'SMLT'
  document.body.className = to.meta.bodyClass || ''
})

export default router
