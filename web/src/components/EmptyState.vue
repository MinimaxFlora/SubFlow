<script setup lang="ts">
// NutUI's <nut-empty> loads its illustrations from a JD CDN, which is
// unreachable on self-hosted deployments (and blocked in some networks), so the
// image slot is filled with original inline artwork instead. Inlining also lets
// the illustration follow the active theme through `currentColor`.
withDefaults(
  defineProps<{
    image?: 'empty' | 'error';
    imageSize?: string | number;
    description?: string;
  }>(),
  { image: 'empty', imageSize: '', description: '' },
);
</script>

<template>
  <nut-empty :image-size="imageSize">
    <template #image>
      <svg
        v-if="image === 'error'"
        class="subflow-empty-illustration"
        viewBox="0 0 240 240"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <circle cx="120" cy="120" r="78" fill="none" stroke="currentColor" stroke-width="10" />
        <path d="M120 78v54" stroke="currentColor" stroke-width="12" stroke-linecap="round" />
        <circle cx="120" cy="160" r="8.5" fill="currentColor" />
      </svg>
      <svg
        v-else
        class="subflow-empty-illustration"
        viewBox="0 0 240 240"
        xmlns="http://www.w3.org/2000/svg"
        aria-hidden="true"
      >
        <path
          d="M48 132 70 78a10 10 0 0 1 9.3-6.2h81.4A10 10 0 0 1 170 78l22 54v36a12 12 0 0 1-12 12H60a12 12 0 0 1-12-12z"
          fill="none"
          stroke="currentColor"
          stroke-width="9"
          stroke-linejoin="round"
        />
        <path
          d="M48 132h41l9 16h44l9-16h41"
          fill="none"
          stroke="currentColor"
          stroke-width="9"
          stroke-linejoin="round"
        />
        <g fill="currentColor">
          <circle cx="84" cy="48" r="7" />
          <circle cx="120" cy="36" r="7" />
          <circle cx="156" cy="48" r="7" />
        </g>
      </svg>
    </template>
    <template #description>
      <slot name="description">
        <div class="nut-empty-description">{{ description }}</div>
      </slot>
    </template>
    <slot />
  </nut-empty>
</template>

<style scoped>
.subflow-empty-illustration {
  width: 100%;
  height: 100%;
  color: currentColor;
  opacity: 0.32;
}
</style>
