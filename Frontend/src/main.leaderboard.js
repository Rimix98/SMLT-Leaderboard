import { createApp } from 'vue'
import { initTheme } from './store'
import LeaderboardPage from './components/LeaderboardPage.vue'

initTheme()
document.documentElement.setAttribute('data-theme', localStorage.getItem('smlt-theme') || 'dark')

const app = createApp(LeaderboardPage)
app.mount('#app')
