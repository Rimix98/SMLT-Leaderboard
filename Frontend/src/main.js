import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { initTheme } from './store'
import { refreshCsrfToken } from './api/utils'

initTheme()
refreshCsrfToken()

const app = createApp(App)
app.use(router)
app.mount('#app')
