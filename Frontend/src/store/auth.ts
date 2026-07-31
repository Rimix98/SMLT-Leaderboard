import { reactive } from 'vue'

export const authStore = reactive({
  isHost: false as boolean,
})

export function setHost(value: boolean): void {
  authStore.isHost = value
}
