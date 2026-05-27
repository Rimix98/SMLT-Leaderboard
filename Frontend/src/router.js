import { ref } from 'vue'

const currentPage = ref('home')

export function useRouter() {
  function navigate(page) {
    currentPage.value = page
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
  return { currentPage, navigate }
}
