import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  { path: '/', name: 'home', component: () => import('./components/HomePage.vue'), meta: { title: 'SMLT - Главная', bodyClass: 'home-page' } },
  { path: '/leaderboard', name: 'leaderboard', component: () => import('./components/LeaderboardPage.vue'), meta: { title: 'Лидерборд -- SMLT', bodyClass: '' } },
  { path: '/projects', name: 'projects', component: () => import('./components/ProjectsPage.vue'), meta: { title: 'Проекты SMLT', bodyClass: '' } },
  { path: '/staff', name: 'staff', component: () => import('./components/StaffPage.vue'), meta: { title: 'Стафф -- SMLT', bodyClass: '' } },
  { path: '/shame-board', name: 'shame-board', component: () => import('./components/ShameBoardPage.vue'), meta: { title: 'Доска позора -- SMLT', bodyClass: '' } },
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
