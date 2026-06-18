import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  build: {
    cssCodeSplit: false,
    emptyOutDir: true,
    sourcemap: false,
  },
  test: {
    globals: true,
    setupFiles: ['src/__tests__/setup.js'],
    include: ['src/**/*.{test,spec}.js'],
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{js,vue}'],
      exclude: ['src/main.js', 'src/va-init.js'],
    },
  },
})
