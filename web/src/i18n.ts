import { computed, ref, watch } from 'vue'
import en from '@/locales/en'
import zhCN from '@/locales/zh-CN'
import zhTW from '@/locales/zh-TW'

export type LocaleCode = 'zh-CN' | 'en' | 'zh-TW'

// Endonyms, so the picker is readable regardless of the active language.
export const supportedLocales: { code: LocaleCode; label: string }[] = [
  { code: 'zh-CN', label: '简体中文' },
  { code: 'en', label: 'English' },
  { code: 'zh-TW', label: '繁體中文' },
]

const catalogs: Record<LocaleCode, Record<string, string>> = {
  'zh-CN': zhCN,
  en,
  'zh-TW': zhTW,
}

const STORAGE_KEY = 'subflow.locale'

function isLocale(value: string | null): value is LocaleCode {
  return value === 'zh-CN' || value === 'en' || value === 'zh-TW'
}

// Match the browser language against the supported locales, defaulting to
// Simplified Chinese because that is the project's primary audience.
function detect(): LocaleCode {
  for (const tag of navigator.languages ?? [navigator.language]) {
    const normalized = tag.toLowerCase()
    if (normalized.startsWith('zh')) {
      return /hant|tw|hk|mo/.test(normalized) ? 'zh-TW' : 'zh-CN'
    }
    if (normalized.startsWith('en')) return 'en'
  }
  return 'zh-CN'
}

function readStored(): LocaleCode {
  const value = localStorage.getItem(STORAGE_KEY)
  return isLocale(value) ? value : detect()
}

export const locale = ref<LocaleCode>(readStored())

// BCP 47 tag used for locale-aware number and date formatting.
export const localeTag = computed(() => locale.value)

export function setLocale(code: LocaleCode) {
  locale.value = code
}

export function cycleLocale() {
  const index = supportedLocales.findIndex((item) => item.code === locale.value)
  locale.value = supportedLocales[(index + 1) % supportedLocales.length].code
}

// t resolves a message key for the active locale, falling back to English and
// then to the key itself. `{name}` placeholders are substituted from params.
export function t(key: string, params?: Record<string, string | number>): string {
  const message = catalogs[locale.value][key] ?? en[key] ?? key
  if (!params) return message
  return message.replace(/\{(\w+)\}/g, (match, name: string) =>
    name in params ? String(params[name]) : match,
  )
}

// translateError maps a known backend error code to a localized message,
// falling back to the message returned by the API.
export function translateError(error: unknown): string {
  if (error instanceof Error) {
    const code = (error as Error & { code?: string }).code
    if (code && `error.${code}` in catalogs[locale.value]) return t(`error.${code}`)
    return error.message
  }
  return String(error)
}

function apply() {
  document.documentElement.lang = locale.value
}

watch(locale, (code) => {
  localStorage.setItem(STORAGE_KEY, code)
  apply()
})

apply()

export function useI18n() {
  return { locale, localeTag, t, setLocale, cycleLocale, supportedLocales }
}
