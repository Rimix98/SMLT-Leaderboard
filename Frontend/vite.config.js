import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  root: '.',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      input: {
        index: resolve(__dirname, 'index.html'),
        leaderboard: resolve(__dirname, 'leaderboard.html'),
        projects: resolve(__dirname, 'projects.html'),
        staff: resolve(__dirname, 'staff.html'),
      }
    }
  }
})
