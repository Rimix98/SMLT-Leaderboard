<script setup lang="ts">
import { useRoute } from 'vue-router'
import { initTheme } from './store'
import { refreshCsrfToken } from './api/utils'

initTheme()
refreshCsrfToken()

const route = useRoute()
</script>

<template>
  <router-view />
</template>
