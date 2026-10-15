<script setup lang="ts">
import { NSwitch } from 'naive-ui'
import Icon from '@/components/Icon.vue'
import type { Subscription } from '@/api/client'

defineProps<{ sub: Subscription }>()

const emit = defineEmits<{
  open: [sub: Subscription]
  edit: [sub: Subscription]
  remove: [sub: Subscription]
  toggle: [sub: Subscription]
  copy: [sub: Subscription]
}>()

function initials(sub: Subscription) {
  const label = (sub.displayName || sub.name || '?').trim()
  return label.slice(0, 2).toUpperCase()
}

function host(sub: Subscription) {
  if (!sub.url) return 'inline content'
  try {
    return new URL(sub.url).host
  } catch {
    return sub.url
  }
}

function updated(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}
</script>

<template>
  <article class="sub-card" :class="{ 'is-off': !sub.enabled }" @click="emit('open', sub)">
    <span class="sub-avatar" :class="{ 'is-local': sub.source === 'local' }">
      {{ initials(sub) }}
    </span>

    <div class="sub-main">
      <div class="sub-head">
        <h3 class="sub-title">
          <span class="name">{{ sub.displayName || sub.name }}</span>
          <span class="tag" :class="sub.source === 'remote' ? 'primary' : ''">{{ sub.source }}</span>
          <span v-if="!sub.enabled" class="tag danger">off</span>
        </h3>

        <div class="sub-menu" @click.stop>
          <button type="button" title="Copy link" @click="emit('copy', sub)">
            <Icon name="copy" :size="16" />
          </button>
          <button type="button" title="Convert" @click="emit('open', sub)">
            <Icon name="convert" :size="16" />
          </button>
          <button type="button" title="Edit" @click="emit('edit', sub)">
            <Icon name="edit" :size="16" />
          </button>
          <button type="button" class="danger" title="Delete" @click="emit('remove', sub)">
            <Icon name="trash" :size="16" />
          </button>
          <n-switch
            size="small"
            :value="sub.enabled"
            style="margin-left: 4px"
            @update:value="emit('toggle', sub)"
          />
        </div>
      </div>

      <p class="sub-detail">
        <span class="row">
          <Icon :name="sub.source === 'remote' ? 'cloud' : 'file'" :size="12" />
          <span>{{ host(sub) }}</span>
        </span>
      </p>
      <p class="sub-remark">
        <span class="row">
          <Icon name="clock" :size="12" />
          <span>{{ updated(sub.updatedAt) }}</span>
        </span>
      </p>
    </div>
  </article>
</template>
