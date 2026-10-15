<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Icon from '@/components/Icon.vue'
import { navItems } from '@/nav'

const route = useRoute()
const router = useRouter()

const activeKey = computed(() => (route.name as string) ?? 'overview')

const width = ref(window.innerWidth)
const expanded = computed(() => width.value >= 1180)

function onResize() {
  width.value = window.innerWidth
}

onMounted(() => window.addEventListener('resize', onResize))
onUnmounted(() => window.removeEventListener('resize', onResize))
</script>

<template>
  <aside class="side-bar-wrapper" :class="{ 'is-expanded': expanded }">
    <div class="menu-items">
      <div
        v-for="item in navItems"
        :key="item.key"
        class="menu-item"
        :class="{ active: activeKey === item.key }"
        :title="item.label"
        @click="router.push({ name: item.key })"
      >
        <Icon :name="item.icon" :size="22" />
        <span class="label">{{ item.label }}</span>
      </div>
    </div>
  </aside>
</template>
