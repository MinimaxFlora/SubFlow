<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type HealthInfo } from '@/api/client'
import { themeMode } from '@/theme'
import Icon from '@/components/Icon.vue'

defineProps<{ title: string; subtitle?: string }>()

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
          :title="health?.status === 'ok' ? 'Backend online' : 'Backend offline'"
        >
          <span class="dot" :class="health?.status === 'ok' ? 'on' : ''" />
        </span>
        <button
          type="button"
          class="nav-icon-btn"
          :title="`Theme: ${themeMode}`"
          :aria-label="`Theme: ${themeMode}`"
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
