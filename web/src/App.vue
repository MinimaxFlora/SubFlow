<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterView, useRoute } from 'vue-router'
import {
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  darkTheme,
  dateEnUS,
  dateZhCN,
  dateZhTW,
  enUS,
  zhCN,
  zhTW,
  type GlobalThemeOverrides,
} from 'naive-ui'
import NavBar from '@/components/NavBar.vue'
import SideBar from '@/components/SideBar.vue'
import TabBar from '@/components/TabBar.vue'
import { api, type VersionInfo } from '@/api/client'
import { isDark } from '@/theme'
import { useI18n } from '@/i18n'

const route = useRoute()
const { locale, t } = useI18n()
const version = ref<VersionInfo | null>(null)

onMounted(async () => {
  try {
    version.value = await api.version()
  } catch {
    version.value = null
  }
})

const pageTitle = computed(() =>
  route.meta.titleKey ? t(route.meta.titleKey as string) : 'SubFlow',
)
const pageSubtitle = computed(() =>
  route.meta.subtitleKey ? t(route.meta.subtitleKey as string) : undefined,
)

// Naive UI ships its own translations for built-in component text.
const naiveLocaleMap = { 'zh-CN': zhCN, en: enUS, 'zh-TW': zhTW } as const
const naiveDateLocaleMap = { 'zh-CN': dateZhCN, en: dateEnUS, 'zh-TW': dateZhTW } as const
const naiveLocale = computed(() => naiveLocaleMap[locale.value])
const naiveDateLocale = computed(() => naiveDateLocaleMap[locale.value])

const overrides = computed<GlobalThemeOverrides>(() => ({
  common: {
    primaryColor: '#478ef2',
    primaryColorHover: '#5c9bf5',
    primaryColorPressed: '#3b7de0',
    primaryColorSuppl: '#3b7de0',
    borderRadius: '10px',
    borderRadiusSmall: '8px',
    fontFamily:
      "'Roboto', 'Noto Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif",
    ...(isDark.value
      ? {
          bodyColor: '#121212',
          cardColor: '#202020',
          modalColor: '#202020',
          popoverColor: '#242424',
          tableColor: '#202020',
          inputColor: '#2b2b2b',
          borderColor: 'rgba(255,255,255,0.10)',
          dividerColor: 'rgba(255,255,255,0.08)',
        }
      : {
          bodyColor: '#f4f4f4',
          cardColor: '#fafafa',
          modalColor: '#f8f8f8',
          popoverColor: '#ffffff',
          inputColor: '#ededed',
          borderColor: 'rgba(0,0,0,0.10)',
          dividerColor: 'rgba(0,0,0,0.06)',
        }),
  },
  Button: { borderRadiusMedium: '10px', borderRadiusSmall: '8px' },
  Input: { borderRadius: '10px' },
  Select: { peers: { InternalSelection: { borderRadius: '10px' } } },
}))
</script>

<template>
  <n-config-provider
    :theme="isDark ? darkTheme : null"
    :theme-overrides="overrides"
    :locale="naiveLocale"
    :date-locale="naiveDateLocale"
  >
    <n-message-provider>
      <n-dialog-provider>
        <NavBar :title="pageTitle" :subtitle="pageSubtitle" />
        <SideBar />

        <main class="page-body">
          <router-view />
          <footer class="page-foot">
            SubFlow
            <span v-if="version?.version && version.version !== 'dev'">v{{ version.version }}</span>
            <span v-if="version?.commit && version.commit !== 'none'">· {{ version.commit.slice(0, 7) }}</span>
          </footer>
        </main>

        <TabBar />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<style scoped>
.page-foot {
  padding: 24px 16px 8px;
  text-align: center;
  font-size: 11.5px;
  color: var(--lowest-text-color);
}
</style>
