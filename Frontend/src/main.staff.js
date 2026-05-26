import { createApp } from 'vue'
import { initTheme } from './store'
import StaffPage from './components/StaffPage.vue'

initTheme()
document.documentElement.setAttribute('data-theme', localStorage.getItem('smlt-theme') || 'dark')

const app = createApp(StaffPage)
app.mount('#app')
