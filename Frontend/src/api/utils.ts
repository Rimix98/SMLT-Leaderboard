export const API_BASE = 'https://api.demonlist.org'
export const BACKEND_URL = '/api'

export const pendingRequests = new Map<string, AbortController>()

export const tokens = {
  csrfToken: '',
  adminKnockKey: '',
}
let adminKnockRefreshTimer: ReturnType<typeof setTimeout> | null = null

function fetchWithTimeout(url: string, opts: RequestInit, ms: number): Promise<Response> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), ms)
  const p = fetch(url, { ...opts, signal: controller.signal })
  p.finally(() => clearTimeout(timeout))
  return p
}

let autoRefreshTimer: ReturnType<typeof setInterval> | null = null

export function startTokenAutoRefresh(): void {
  if (autoRefreshTimer) return
  autoRefreshTimer = setInterval(async () => {
    try {
      const res = await fetchWithTimeout(`${BACKEND_URL}/auth/refresh`, {
        method: 'POST',
        credentials: 'include',
      }, 10000)
      if (res.status === 401) {
        const { store } = await import('../store')
        store.isHost = false
        stopTokenAutoRefresh()
      }
    } catch { /* ignore */ }
  }, 30 * 60 * 1000)
}

export function stopTokenAutoRefresh(): void {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

export async function refreshCsrfToken(): Promise<string | null> {
  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      const res = await fetchWithTimeout(`${BACKEND_URL}/csrf-token`, { credentials: 'include' }, 10000)
      const data = await res.json()
      if (data.token) tokens.csrfToken = data.token
      return data.token || null
    } catch (e) {
      if (attempt === 0) { await new Promise(r => setTimeout(r, 1000)); continue }
      console.error('Не удалось получить CSRF токен:', e)
      return null
    }
  }
  return null
}

export async function doAdminKnock(): Promise<boolean> {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/knock-knock-admin`, {
      method: 'POST',
      credentials: 'include'
    }, 'admin-knock')
    if (!res.ok) return false
    const data = await parseJsonResponse(res)
    if (data && data.key) {
      tokens.adminKnockKey = data.key as string
      if (adminKnockRefreshTimer) clearTimeout(adminKnockRefreshTimer)
      const ttl = ((data.ttl as number) || 900) * 1000
      adminKnockRefreshTimer = setTimeout(() => doAdminKnock(), ttl - 60000)
      return true
    }
    return false
  } catch {
    return false
  }
}

interface FetchOptions extends RequestInit {
  timeout?: number
}

export async function fetchWithAbort(url: string, options: FetchOptions = {}, key: string | null = null): Promise<Response> {
  if (key && pendingRequests.has(key)) {
    pendingRequests.get(key)!.abort()
  }
  const controller = new AbortController()
  if (key) pendingRequests.set(key, controller)

  const buildHeaders = (): Record<string, string> => {
    const h: Record<string, string> = { ...(options.headers as Record<string, string>), 'X-Requested-With': 'XMLHttpRequest' }
    if (['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method?.toUpperCase() || '')) {
      if (tokens.csrfToken) h['X-CSRF-Token'] = tokens.csrfToken
      if (tokens.adminKnockKey) h['X-Admin-Path-Key'] = tokens.adminKnockKey
    }
    return h
  }

  const timeoutMs = options.timeout || 30000
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs)

  const cleanup = () => {
    clearTimeout(timeoutId)
    if (key && pendingRequests.get(key) === controller) {
      pendingRequests.delete(key)
    }
  }

  try {
    let headers = buildHeaders()
    let res: Response | null = null
    try {
      res = await fetch(url, { ...options, headers, signal: controller.signal })
    } catch (fetchErr) {
      cleanup()
      throw fetchErr
    }

    const newCsrf = res.headers.get('X-CSRF-Token')
    if (newCsrf) tokens.csrfToken = newCsrf

    if (!res.ok && res.status === 404 && ['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method?.toUpperCase() || '')) {
      const cloned = res.clone()
      const text = await cloned.text().catch(() => '')
      if (text.includes('Роут не найден')) {
        tokens.adminKnockKey = ''
        const knocked = await doAdminKnock()
        if (knocked) {
          headers = buildHeaders()
          const controller2 = new AbortController()
          const timeoutId2 = setTimeout(() => controller2.abort(), timeoutMs)
          try {
            res = await fetch(url, { ...options, headers, signal: controller2.signal })
          } finally {
            clearTimeout(timeoutId2)
          }
        }
      }
    }

    if (!res.ok && res.status === 403 && ['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method?.toUpperCase() || '')) {
      tokens.csrfToken = ''
      const refreshed = await refreshCsrfToken()
      if (refreshed) {
        headers = buildHeaders()
        const controller2 = new AbortController()
        const timeoutId2 = setTimeout(() => controller2.abort(), timeoutMs)
        try {
          res = await fetch(url, { ...options, headers, signal: controller2.signal })
          const newCsrf = res.headers.get('X-CSRF-Token')
          if (newCsrf) tokens.csrfToken = newCsrf
        } finally {
          clearTimeout(timeoutId2)
        }
      }
    }

    return res
  } finally {
    cleanup()
  }
}

export function isAbortError(err: unknown): boolean {
  return err instanceof Error && err.name === 'AbortError'
}

export async function parseJsonResponse(res: Response): Promise<Record<string, unknown>> {
  const contentType = res.headers.get('content-type') || ''
  const text = await res.text()
  if (!text) return {}
  if (!contentType.includes('application/json')) {
    if (text.trimStart().startsWith('<')) {
      throw new Error('API недоступен (ошибка сервера). Проверьте переменные окружения на Vercel.')
    }
    throw new Error('Сервер вернул некорректный ответ')
  }
  try {
    return JSON.parse(text) as Record<string, unknown>
  } catch (e) {
    console.error('Ошибка парсинга JSON:', e, text.slice(0, 200))
    throw new Error('Сервер вернул некорректный ответ')
  }
}

// Flags & countries
export const FLAGS: Record<string, string> = {
  'RU': '\u{1F1F7}\u{1F1FA}', 'US': '\u{1F1FA}\u{1F1F8}', 'DE': '\u{1F1E9}\u{1F1EA}', 'FR': '\u{1F1EB}\u{1F1F7}', 'GB': '\u{1F1EC}\u{1F1E7}',
  'BR': '\u{1F1E7}\u{1F1F7}', 'KR': '\u{1F1F0}\u{1F1F7}', 'JP': '\u{1F1EF}\u{1F1F5}', 'CN': '\u{1F1E8}\u{1F1F3}', 'PL': '\u{1F1F5}\u{1F1F1}',
  'UA': '\u{1F1FA}\u{1F1E6}', 'CA': '\u{1F1E8}\u{1F1E6}', 'AU': '\u{1F1E6}\u{1F1FA}', 'ES': '\u{1F1EA}\u{1F1F8}', 'IT': '\u{1F1EE}\u{1F1F9}',
  'AR': '\u{1F1E6}\u{1F1F7}', 'CL': '\u{1F1E8}\u{1F1F1}', 'MX': '\u{1F1F2}\u{1F1FD}', 'NL': '\u{1F1F3}\u{1F1F1}', 'SE': '\u{1F1F8}\u{1F1EA}',
  'NO': '\u{1F1F3}\u{1F1F4}', 'FI': '\u{1F1EB}\u{1F1EE}', 'DK': '\u{1F1E9}\u{1F1F0}', 'BE': '\u{1F1E7}\u{1F1EA}', 'AT': '\u{1F1E6}\u{1F1F9}',
  'CZ': '\u{1F1E8}\u{1F1FF}', 'SK': '\u{1F1F8}\u{1F1F0}', 'HU': '\u{1F1ED}\u{1F1FA}', 'RO': '\u{1F1F7}\u{1F1F4}', 'BG': '\u{1F1E7}\u{1F1EC}',
  'TR': '\u{1F1F9}\u{1F1F7}', 'IL': '\u{1F1EE}\u{1F1F1}', 'SA': '\u{1F1F8}\u{1F1E6}', 'AE': '\u{1F1E6}\u{1F1EA}', 'IN': '\u{1F1EE}\u{1F1F3}',
  'ID': '\u{1F1EE}\u{1F1E9}', 'TH': '\u{1F1F9}\u{1F1ED}', 'VN': '\u{1F1FB}\u{1F1F3}', 'MY': '\u{1F1F2}\u{1F1FE}', 'SG': '\u{1F1F8}\u{1F1EC}',
  'PH': '\u{1F1F5}\u{1F1ED}', 'NZ': '\u{1F1F3}\u{1F1FF}', 'ZA': '\u{1F1FF}\u{1F1E6}', 'EG': '\u{1F1EA}\u{1F1EC}', 'NG': '\u{1F1F3}\u{1F1EC}',
  'CO': '\u{1F1E8}\u{1F1F4}', 'PE': '\u{1F1F5}\u{1F1EA}', 'VE': '\u{1F1FB}\u{1F1EA}', 'EC': '\u{1F1EA}\u{1F1E8}', 'PT': '\u{1F1F5}\u{1F1F9}',
  'GR': '\u{1F1EC}\u{1F1F7}', 'HR': '\u{1F1ED}\u{1F1F7}', 'RS': '\u{1F1F7}\u{1F1F8}', 'SI': '\u{1F1F8}\u{1F1EE}', 'EE': '\u{1F1EA}\u{1F1EA}',
  'LV': '\u{1F1F1}\u{1F1FB}', 'LT': '\u{1F1F1}\u{1F1F9}', 'BY': '\u{1F1E7}\u{1F1FE}', 'KZ': '\u{1F1F0}\u{1F1FF}', 'UZ': '\u{1F1FA}\u{1F1FF}',
  'TW': '\u{1F1F9}\u{1F1FC}', 'HK': '\u{1F1ED}\u{1F1F0}', 'MO': '\u{1F1F2}\u{1F1F4}', 'AM': '\u{1F1E6}\u{1F1F2}', 'MD': '\u{1F1F2}\u{1F1E9}'
}

export const COUNTRY_TO_CODE: Record<string, string> = {
  'russia': 'RU', 'russian-federation': 'RU',
  'united-states': 'US', 'united-states-of-america': 'US', 'usa': 'US',
  'germany': 'DE', 'france': 'FR',
  'united-kingdom': 'GB', 'great-britain': 'GB', 'uk': 'GB',
  'brazil': 'BR', 'south-korea': 'KR', 'korea': 'KR', 'north-korea': 'KP',
  'japan': 'JP', 'china': 'CN', 'poland': 'PL', 'ukraine': 'UA',
  'canada': 'CA', 'australia': 'AU', 'spain': 'ES', 'italy': 'IT',
  'argentina': 'AR', 'chile': 'CL', 'mexico': 'MX', 'netherlands': 'NL', 'holland': 'NL',
  'sweden': 'SE', 'norway': 'NO', 'finland': 'FI', 'denmark': 'DK',
  'belgium': 'BE', 'austria': 'AT', 'czech-republic': 'CZ', 'czechia': 'CZ',
  'slovakia': 'SK', 'hungary': 'HU', 'romania': 'RO', 'bulgaria': 'BG',
  'turkey': 'TR', 'israel': 'IL', 'saudi-arabia': 'SA', 'united-arab-emirates': 'AE',
  'india': 'IN', 'indonesia': 'ID', 'thailand': 'TH', 'vietnam': 'VN',
  'malaysia': 'MY', 'singapore': 'SG', 'philippines': 'PH', 'new-zealand': 'NZ',
  'south-africa': 'ZA', 'egypt': 'EG', 'nigeria': 'NG', 'colombia': 'CO',
  'peru': 'PE', 'venezuela': 'VE', 'ecuador': 'EC', 'portugal': 'PT',
  'greece': 'GR', 'croatia': 'HR', 'serbia': 'RS', 'slovenia': 'SI',
  'estonia': 'EE', 'latvia': 'LV', 'lithuania': 'LT', 'belarus': 'BY',
  'kazakhstan': 'KZ', 'uzbekistan': 'UZ', 'taiwan': 'TW', 'hong-kong': 'HK',
  'macau': 'MO', 'armenia': 'AM', 'moldova': 'MD'
}

export const CODE_TO_NAME: Record<string, string> = {
  'RU': 'Россия', 'US': 'США', 'DE': 'Германия', 'FR': 'Франция',
  'GB': 'Великобритания', 'BR': 'Бразилия', 'KR': 'Южная Корея',
  'JP': 'Япония', 'CN': 'Китай', 'PL': 'Польша', 'UA': 'Украина',
  'CA': 'Канада', 'AU': 'Австралия', 'ES': 'Испания', 'IT': 'Италия',
  'AR': 'Аргентина', 'CL': 'Чили', 'MX': 'Мексика', 'NL': 'Нидерланды',
  'SE': 'Швеция', 'NO': 'Норвегия', 'FI': 'Финляндия', 'DK': 'Дания',
  'BE': 'Бельгия', 'AT': 'Австрия', 'CZ': 'Чехия', 'SK': 'Словакия',
  'HU': 'Венгрия', 'RO': 'Румыния', 'BG': 'Болгария', 'TR': 'Турция',
  'IL': 'Израиль', 'SA': 'Саудовская Аравия', 'AE': 'ОАЭ', 'IN': 'Индия',
  'ID': 'Индонезия', 'TH': 'Таиланд', 'VN': 'Вьетнам', 'MY': 'Малайзия',
  'SG': 'Сингапур', 'PH': 'Филиппины', 'NZ': 'Новая Зеландия',
  'ZA': 'ЮАР', 'EG': 'Египет', 'NG': 'Нигерия', 'CO': 'Колумбия',
  'PE': 'Перу', 'VE': 'Венесуэла', 'EC': 'Эквадор', 'PT': 'Португалия',
  'GR': 'Греция', 'HR': 'Хорватия', 'RS': 'Сербия', 'SI': 'Словения',
  'EE': 'Эстония', 'LV': 'Латвия', 'LT': 'Литва', 'BY': 'Беларусь',
  'KZ': 'Казахстан', 'UZ': 'Узбекистан', 'TW': 'Тайвань',
  'HK': 'Гонконг', 'MO': 'Макао', 'AM': 'Армения', 'MD': 'Молдова'
}

export function resolveCountry(input: string | null): string | null {
  if (!input) return null
  let val = input.toString().trim()
  const upper = val.toUpperCase()
  if (FLAGS[upper]) return upper
  val = val.toLowerCase()
  val = val.replace(/\s*\(.*?\)\s*/g, ' ').trim()
  val = val.replace(/\s+(federation|republic|island|territory|kingdom|principality|emirate|commonwealth|union|state|states|region|province|of|the|and|islands)$/gi, '').trim()
  val = val.replace(/\s+/g, '-')
  const mapped = COUNTRY_TO_CODE[val]
  if (mapped) return mapped
  const fallback = input.toString().toLowerCase().trim().replace(/\s*\(.*?\)\s*/g, ' ').trim().replace(/\s+/g, '-')
  return COUNTRY_TO_CODE[fallback] || null
}

function isValidISOCode(code: string | null): code is string {
  return typeof code === 'string' && /^[A-Z]{2}$/.test(code)
}

export function getFlagCode(c: string | null | undefined): string | null {
  if (!c) return null
  const code = resolveCountry(c)
  if (!code || !isValidISOCode(code)) return null
  return code.toLowerCase()
}

export function showToast(msg: string, type = 'error'): void {
  const t = document.createElement('div')
  t.className = `toast toast-${type}`
  t.textContent = msg
  const container = document.getElementById('toastContainer')
  if (container) container.appendChild(t)
  setTimeout(() => t.remove(), 5000)
}
