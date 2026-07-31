import { reactive } from 'vue'
import type { SMPStatus } from '../types'

export const smpStore = reactive({
  status: null as SMPStatus | null,
})
