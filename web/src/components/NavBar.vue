<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, type HealthInfo } from '@/api/client'
import { themeMode } from '@/theme'
import { cycleLocale, supportedLocales, useI18n } from '@/i18n'
import Icon from '@/components/Icon.vue'

defineProps<{ title: string; subtitle?: string }>()

const { locale, t } = useI18n()
const health = ref<HealthInfo | null>(null)

onMounted(async () => {
  try {
    health.value = await api.health()
  } catch {
    health.value = null
  }
})

const modes = ['auto', 'light', 'dark'] as const
const modeIcon: Record<string, string> = { auto: 'monitor', light: 'sun', dark: 'moon' }
const modeLabel = computed(() => t(`theme.${themeMode.value}`))
const localeLabel = computed(
  () => supportedLocales.find((item) => item.code === locale.value)?.label ?? locale.value,
)

function cycleTheme() {
  const index = modes.indexOf(themeMode.value)
  themeMode.value = modes[(index + 1) % modes.length]
}
</script>

<template>
  <div class="nav-bar-wrapper">
    <div class="nav-bar">
      <div class="nav-bar__leading">
        <slot name="leading" />
      </div>

      <div class="nav-bar__title">
        <span>{{ title }}</span>
        <span v-if="subtitle" class="sub">{{ subtitle }}</span>
      </div>

      <div class="nav-bar__trailing">
        <slot name="trailing" />
        <span
          class="nav-icon-btn"
          :title="health?.status === 'ok' ? t('navBar.backendOnline') : t('navBar.backendOffline')"
        >
          <span class="dot" :class="health?.status === 'ok' ? 'on' : ''" />
        </span>
        <button
          type="button"
          class="nav-icon-btn"
          :title="t('navBar.language', { language: localeLabel })"
          :aria-label="t('navBar.language', { language: localeLabel })"
          @click="cycleLocale"
        >
          <Icon name="globe" :size="17" />
        </button>
        <button
          type="button"
          class="nav-icon-btn"
          :title="t('navBar.theme', { mode: modeLabel })"
          :aria-label="t('navBar.theme', { mode: modeLabel })"
          @click="cycleTheme"
        >
          <Icon :name="modeIcon[themeMode]" :size="17" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.nav-icon-btn .dot {
  width: 9px;
  height: 9px;
}
</style>
