<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Icon from '@/components/Icon.vue'
import { navItems } from '@/nav'
import { useI18n } from '@/i18n'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const activeKey = computed(() => (route.name as string) ?? 'overview')
</script>

<template>
  <div class="tab-bar-wrapper">
    <nav class="tab-bar">
      <button
        v-for="item in navItems"
        :key="item.key"
        type="button"
        class="tab-item"
        :class="{ active: activeKey === item.key }"
        @click="router.push({ name: item.key })"
      >
        <Icon :name="item.icon" :size="22" />
        <span>{{ t(item.labelKey) }}</span>
      </button>
    </nav>
  </div>
</template>
