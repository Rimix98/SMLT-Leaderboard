export const API_BASE = 'https://api.demonlist.org'
export const BACKEND_URL = '/api'

export const pendingRequests = new Map()

export const tokens = {
  csrfToken: '',
  adminKnockKey: '',
}
let adminKnockRefreshTimer = null

function fetchWithTimeout(url, opts, ms) {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), ms)
  const p = fetch(url, { ...opts, signal: controller.signal })
  p.finally(() => clearTimeout(timeout))
  return p
}

export async function refreshCsrfToken() {
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

export async function doAdminKnock() {
  try {
    const res = await fetchWithAbort(`${BACKEND_URL}/knock-knock-admin`, {
      method: 'POST',
      credentials: 'include'
    }, 'admin-knock')
    if (!res.ok) return false
    const data = await parseJsonResponse(res)
    if (data && data.key) {
      tokens.adminKnockKey = data.key
      if (adminKnockRefreshTimer) clearTimeout(adminKnockRefreshTimer)
      const ttl = (data.ttl || 900) * 1000
      adminKnockRefreshTimer = setTimeout(() => doAdminKnock(), ttl - 60000)
      return true
    }
    return false
  } catch {
    return false
  }
}

export async function fetchWithAbort(url, options = {}, key = null) {
  if (key && pendingRequests.has(key)) {
    pendingRequests.get(key).abort()
  }
  const controller = new AbortController()
  if (key) pendingRequests.set(key, controller)

  const buildHeaders = () => {
    const h = { ...options.headers, 'X-Requested-With': 'XMLHttpRequest' }
    if (['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method?.toUpperCase())) {
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
    let res = null
    try {
      res = await fetch(url, { ...options, headers, signal: controller.signal })
    } catch (fetchErr) {
      cleanup()
      throw fetchErr
    }

    const newCsrf = res.headers.get('X-CSRF-Token')
    if (newCsrf) tokens.csrfToken = newCsrf

    if (!res.ok && res.status === 404 && ['POST', 'PUT', 'DELETE', 'PATCH'].includes(options.method?.toUpperCase())) {
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

    return res
  } finally {
    cleanup()
  }
}

export function isAbortError(err) {
  return err?.name === 'AbortError'
}

export async function parseJsonResponse(res) {
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
    return JSON.parse(text)
  } catch (e) {
    console.error('Ошибка парсинга JSON:', e, text.slice(0, 200))
    throw new Error('Сервер вернул некорректный ответ')
  }
}

// Flags & countries
export const FLAGS = {
  'RU': '🇷🇺', 'US': '🇺🇸', 'DE': '🇩🇪', 'FR': '🇫🇷', 'GB': '🇬🇧',
  'BR': '🇧🇷', 'KR': '🇰🇷', 'JP': '🇯🇵', 'CN': '🇨🇳', 'PL': '🇵🇱',
  'UA': '🇺🇦', 'CA': '🇨🇦', 'AU': '🇦🇺', 'ES': '🇪🇸', 'IT': '🇮🇹',
  'AR': '🇦🇷', 'CL': '🇨🇱', 'MX': '🇲🇽', 'NL': '🇳🇱', 'SE': '🇸🇪',
  'NO': '🇳🇴', 'FI': '🇫🇮', 'DK': '🇩🇰', 'BE': '🇧🇪', 'AT': '🇦🇹',
  'CZ': '🇨🇿', 'SK': '🇸🇰', 'HU': '🇭🇺', 'RO': '🇷🇴', 'BG': '🇧🇬',
  'TR': '🇹🇷', 'IL': '🇮🇱', 'SA': '🇸🇦', 'AE': '🇦🇪', 'IN': '🇮🇳',
  'ID': '🇮🇩', 'TH': '🇹🇭', 'VN': '🇻🇳', 'MY': '🇲🇾', 'SG': '🇸🇬',
  'PH': '🇵🇭', 'NZ': '🇳🇿', 'ZA': '🇿🇦', 'EG': '🇪🇬', 'NG': '🇳🇬',
  'CO': '🇨🇴', 'PE': '🇵🇪', 'VE': '🇻🇪', 'EC': '🇪🇨', 'PT': '🇵🇹',
  'GR': '🇬🇷', 'HR': '🇭🇷', 'RS': '🇷🇸', 'SI': '🇸🇮', 'EE': '🇪🇪',
  'LV': '🇱🇻', 'LT': '🇱🇹', 'BY': '🇧🇾', 'KZ': '🇰🇿', 'UZ': '🇺🇿',
  'TW': '🇹🇼', 'HK': '🇭🇰', 'MO': '🇲🇴', 'AM': '🇦🇲', 'MD': '🇲🇩'
}

export const COUNTRY_TO_CODE = {
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

export const CODE_TO_NAME = {
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

export function resolveCountry(input) {
  if (!input) return null
  let val = input.toString().trim()
  const upper = val.toUpperCase()
  if (FLAGS[upper]) return upper
  val = val.toLowerCase()
  // strip parentheticals: "Russia (Russian Federation)" → "Russia"
  val = val.replace(/\s*\(.*?\)\s*/g, ' ').trim()
  // strip common suffixes for fuzzy matching
  val = val.replace(/\s+(federation|republic|island|territory|kingdom|principality|emirate|commonwealth|union|state|states|region|province|of|the|and|islands)$/gi, '').trim()
  val = val.replace(/\s+/g, '-')
  const mapped = COUNTRY_TO_CODE[val]
  if (mapped) return mapped
  // retry with full normalized string (without stripping suffixes)
  const fallback = input.toString().toLowerCase().trim().replace(/\s*\(.*?\)\s*/g, ' ').trim().replace(/\s+/g, '-')
  return COUNTRY_TO_CODE[fallback] || null
}

function isValidISOCode(code) {
  return typeof code === 'string' && /^[A-Z]{2}$/.test(code)
}

export function getFlagHTML(c) {
  const code = resolveCountry(c)
  if (!code || !isValidISOCode(code)) return !code && c === null ? '❌' : '🌍'
  return `<img src="https://flagcdn.com/w20/${code.toLowerCase()}.png" alt="${code}" width="20" style="vertical-align:middle;border-radius:2px;margin-right:4px">`
}

export function createFlagElement(c) {
  const code = resolveCountry(c)
  if (!code || !isValidISOCode(code)) {
    const span = document.createElement('span')
    span.textContent = !code && c === null ? '❌' : '🌍'
    return span
  }
  const img = document.createElement('img')
  img.src = `https://flagcdn.com/w20/${code.toLowerCase()}.png`
  img.alt = code
  img.width = 20
  img.style.cssText = 'vertical-align:middle;border-radius:2px;margin-right:4px'
  return img
}

export function getCountryLabel(c) {
  if (!c) return 'неизвестно'
  const upper = c.toUpperCase()
  if (FLAGS[upper]) return upper
  const lower = c.toLowerCase().trim().replace(/\s+/g, '-')
  const code = COUNTRY_TO_CODE[lower]
  if (code) return CODE_TO_NAME[code] || code
  return c
}

export function encodeCountryToken(country) {
  const code = resolveCountry(country)
  if (!code) return ''
  return btoa(encodeURIComponent(code))
}

export function decodeCountryToken(token) {
  try { return decodeURIComponent(atob(token)) } catch { return '' }
}

// Toast
export function showToast(msg, type = 'error') {
  const t = document.createElement('div')
  t.className = `toast toast-${type}`
  t.textContent = msg
  const container = document.getElementById('toastContainer')
  if (container) container.appendChild(t)
  setTimeout(() => t.remove(), 5000)
}
