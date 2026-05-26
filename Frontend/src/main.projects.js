import { createApp } from 'vue'
import { initTheme } from './store'
import ProjectsPage from './components/ProjectsPage.vue'

initTheme()
document.documentElement.setAttribute('data-theme', localStorage.getItem('smlt-theme') || 'dark')

const app = createApp(ProjectsPage)
app.mount('#app')
