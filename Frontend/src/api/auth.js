import { store } from '../store'
import { fetchWithAbort, parseJsonResponse, isAbortError, BACKEND_URL, refreshCsrfToken, doAdminKnock, startTokenAutoRefresh, stopTokenAutoRefresh, showToast } from './utils'

export { refreshCsrfToken, doAdminKnock }

let captchaId = ''

export async function initCaptcha() {
  const img = document.getElementById('captcha-img')
  const input = document.getElementById('captchaInput')
  if (!img) return

  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/captcha`, { credentials: 'include' }, 'captcha-fetch')
    const data = await parseJsonResponse(res)
    if (!res.ok || !data.captchaId) {
      console.error('Ошибка получения капчи')
      return
    }
    captchaId = data.captchaId
    img.src = data.captchaImage
    img.style.display = 'block'
    if (input) input.value = ''
  } catch (err) {
    if (isAbortError(err)) return
    console.error('Ошибка загрузки капчи:', err)
  }
}

export async function initHostStatus() {
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

export function showHostModal() {
  const modal = document.getElementById('hostModal')
  const passwordInput = document.getElementById('hostPassword')
  const errorEl = document.getElementById('hostError')

  if (modal) {
    modal.classList.add('active')
    if (passwordInput) {
      passwordInput.value = ''
      passwordInput.focus()
    }
    if (errorEl) errorEl.style.display = 'none'
    initCaptcha()
  }
}

export function closeHostModal() {
  const modal = document.getElementById('hostModal')
  if (modal) modal.classList.remove('active')
}

export async function verifyHost(inputPassword) {
  const captchaInput = document.getElementById('captchaInput')

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
      const errorMsg = data.error || 'Неверный пароль хоста!'
      showToast(errorMsg, 'error')
      store.isHost = false
      initCaptcha()
    }
  } catch (err) {
    if (isAbortError(err)) return
    console.error('Ошибка входа:', err)
    showToast(
      err.message === 'Сервер вернул некорректный ответ'
        ? 'Ошибка сервера: некорректный формат данных'
        : 'Ошибка соединения с сервером. Проверьте сеть или статус сервера.',
      'error'
    )
    initCaptcha()
  }
}

export async function logoutHost() {
  store.isHost = false
  stopTokenAutoRefresh()
  try {
    await fetchWithAbort(`${BACKEND_URL}/logout`, { method: 'POST', credentials: 'include' }, 'host-logout')
  } catch (e) {
    if (!isAbortError(e)) console.error('Не удалось разлогиниться на сервере', e)
  }
  showToast('Вы вышли из режима хоста', 'info')
}
