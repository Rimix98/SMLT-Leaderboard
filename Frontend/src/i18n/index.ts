import { reactive, computed } from 'vue'
import { createI18n } from 'vue-i18n'
import ru from './locales/ru.json'
import en from './locales/en.json'
import type { LocaleCode } from '../types'

const STORAGE_KEY = 'smlt-locale'

function detectInitialLocale(): LocaleCode {
  if (typeof localStorage === 'undefined') return 'ru'
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved === 'ru' || saved === 'en') return saved
  if (typeof navigator !== 'undefined') {
    const lang = navigator.language?.toLowerCase() || ''
    if (lang.startsWith('en')) return 'en'
  }
  return 'ru'
}

export const localeState = reactive({
  current: detectInitialLocale() as LocaleCode,
})

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: localeState.current,
  fallbackLocale: 'ru',
  messages: { ru, en },
})

export const availableLocales: { code: LocaleCode; nativeLabel: string }[] = [
  { code: 'ru', nativeLabel: 'Русский' },
  { code: 'en', nativeLabel: 'English' },
]

export function setLocale(code: LocaleCode): void {
  if (code !== 'ru' && code !== 'en') return
  localeState.current = code
  i18n.global.locale.value = code
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('lang', code)
  }
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(STORAGE_KEY, code)
  }
}

export const t = (key: string, params?: Record<string, string | number>) =>
  i18n.global.t(key, params ?? {})

export const currentLocale = computed<LocaleCode>(() => localeState.current)

export const languageDirection = computed<'ltr' | 'rtl'>(() => 'ltr')
