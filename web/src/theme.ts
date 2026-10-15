import { ref, watch } from 'vue'

export type ThemeMode = 'auto' | 'light' | 'dark'

const STORAGE_KEY = 'subflow.theme'
const media = window.matchMedia('(prefers-color-scheme: dark)')

function readStored(): ThemeMode {
  const value = localStorage.getItem(STORAGE_KEY)
  return value === 'light' || value === 'dark' || value === 'auto' ? value : 'auto'
}

export const themeMode = ref<ThemeMode>(readStored())
export const isDark = ref(false)

function resolve(mode: ThemeMode) {
  return mode === 'auto' ? media.matches : mode === 'dark'
}

function apply() {
  const dark = resolve(themeMode.value)
  isDark.value = dark
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
}

// Keep the document in sync with the selected mode and, for `auto`, the OS setting.
watch(themeMode, (mode) => {
  localStorage.setItem(STORAGE_KEY, mode)
  apply()
})
media.addEventListener('change', () => {
  if (themeMode.value === 'auto') apply()
})

apply()

export function useTheme() {
  return { themeMode, isDark }
}
