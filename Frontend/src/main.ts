import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { initTheme } from './store'

initTheme()

const app = createApp(App)
app.use(router)
app.mount('#app')
