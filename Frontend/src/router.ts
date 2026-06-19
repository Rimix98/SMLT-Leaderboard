import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import HomePage from './components/HomePage.vue'
import LeaderboardPage from './components/LeaderboardPage.vue'
import ProjectsPage from './components/ProjectsPage.vue'
import StaffPage from './components/StaffPage.vue'

const routes: RouteRecordRaw[] = [
  { path: '/', name: 'home', component: HomePage, meta: { title: 'SMLT - Главная', bodyClass: 'home-page' } },
  { path: '/leaderboard', name: 'leaderboard', component: LeaderboardPage, meta: { title: 'Лидерборд -- SMLT', bodyClass: '' } },
  { path: '/projects', name: 'projects', component: ProjectsPage, meta: { title: 'Проекты SMLT', bodyClass: '' } },
  { path: '/staff', name: 'staff', component: StaffPage, meta: { title: 'Стафф -- SMLT', bodyClass: '' } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(_to, _from, savedPosition) {
    if (savedPosition) return savedPosition
    return { top: 0, behavior: 'smooth' }
  },
})

router.afterEach((to) => {
  document.title = (to.meta.title as string) || 'SMLT'
  document.body.className = (to.meta.bodyClass as string) || ''
})

export default router
