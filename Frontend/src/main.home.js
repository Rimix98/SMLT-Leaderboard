import { createApp } from 'vue'
import { initTheme } from './store'
import HomePage from './components/HomePage.vue'

initTheme()
document.documentElement.setAttribute('data-theme', localStorage.getItem('smlt-theme') || 'dark')

const app = createApp(HomePage)
app.mount('#app')
