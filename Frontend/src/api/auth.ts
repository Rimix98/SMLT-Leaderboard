import { store } from '../store'
import { fetchWithAbort, parseJsonResponse, isAbortError, BACKEND_URL, refreshCsrfToken, doAdminKnock, startTokenAutoRefresh, stopTokenAutoRefresh, showToast } from './utils'

export { refreshCsrfToken, doAdminKnock }

let captchaId = ''

export async function initCaptcha(): Promise<void> {
  const img = document.getElementById('captcha-img') as HTMLImageElement | null
  const input = document.getElementById('captchaInput') as HTMLInputElement | null
  if (!img) return

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/captcha`, { credentials: 'include' }, 'captcha-fetch')
    const data = await parseJsonResponse(res)
    if (!res.ok || !data.captchaId) {
      showToast('Не удалось загрузить капчу', 'error')
      return
    }
    captchaId = data.captchaId as string
    img.src = data.captchaImage as string
    img.style.display = 'block'
    if (input) input.value = ''
  } catch (err) {
    if (isAbortError(err)) return
    showToast('Ошибка загрузки капчи', 'error')
  }
}

export async function initHostStatus(): Promise<void> {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/verify`, { credentials: 'include' }, 'auth-verify')
    const data = await parseJsonResponse(res)
    store.isHost = res.ok && data.success === true
    if (store.isHost) {
      await doAdminKnock()
      startTokenAutoRefresh()
    }
  } catch (err) {
    if (isAbortError(err)) return
    store.isHost = false
  }
}

export async function verifyHost(inputPassword: string): Promise<void> {
  const captchaInput = document.getElementById('captchaInput') as HTMLInputElement | null

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({
        password: inputPassword,
        captchaId: captchaId,
        captchaValue: captchaInput ? captchaInput.value : ''
      })
    }, 'host-login')

    const data = await parseJsonResponse(res)

    if (res.ok && data.success === true) {
      store.isHost = true
      await doAdminKnock()
      startTokenAutoRefresh()
      showToast('Доступ предоставлен! Вы вошли как хост.', 'success')

      const modal = document.getElementById('hostModal')
      if (modal) modal.classList.remove('active')
    } else {
      const errorMsg = (data.error as string) || 'Неверный пароль хоста!'
      showToast(errorMsg, 'error')
      store.isHost = false
      initCaptcha()
    }
  } catch (err) {
    if (isAbortError(err)) return
    console.error('Ошибка входа:', err)
    showToast(
      (err instanceof Error && err.message) === 'Сервер вернул некорректный ответ'
        ? 'Ошибка сервера: некорректный формат данных'
        : 'Ошибка соединения с сервером. Проверьте сеть или статус сервера.',
      'error'
    )
    initCaptcha()
  }
}

export async function logoutHost(): Promise<void> {
  store.isHost = false
  stopTokenAutoRefresh()
  try {
    await fetchWithAbort(`${BACKEND_URL}/logout`, { method: 'POST', credentials: 'include' }, 'host-logout')
  } catch (e) {
    if (!isAbortError(e)) console.error('Не удалось разлогиниться на сервере', e)
  }
  showToast('Вы вышли из режима хоста', 'info')
}
