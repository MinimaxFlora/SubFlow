<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NSpin, useMessage } from 'naive-ui'
import Icon from '@/components/Icon.vue'
import { api, type Platform, type Subscription } from '@/api/client'
import { translateError, useI18n } from '@/i18n'

const router = useRouter()
const message = useMessage()
const { t } = useI18n()
const loading = ref(true)
const platforms = ref<Platform[]>([])
const subscriptions = ref<Subscription[]>([])

const protocols = ['VLESS', 'VMess', 'Trojan', 'Shadowsocks', 'Hysteria2', 'TUIC']

onMounted(async () => {
  try {
    const [platformList, subs] = await Promise.all([api.listPlatforms(), api.listSubscriptions()])
    platforms.value = platformList
    subscriptions.value = subs
  } catch (err) {
    message.error(translateError(err))
  } finally {
    loading.value = false
  }
})

const groupedPlatforms = computed(() => {
  const groups = new Map<string, Platform[]>()
  for (const p of platforms.value) {
    const list = groups.get(p.group) ?? []
    list.push(p)
    groups.set(p.group, list)
  }
  return [...groups.entries()].map(([group, items]) => ({
    group,
    items: items.slice().sort((a, b) => a.name.localeCompare(b.name)),
  }))
})

const activeSubs = computed(() => subscriptions.value.filter((s) => s.enabled).length)
const recent = computed(() =>
  subscriptions.value.slice().sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)).slice(0, 4),
)
</script>

<template>
  <n-spin :show="loading">
    <div class="views">
      <section class="banner card">
        <div class="banner-copy">
          <span class="banner-eyebrow">
            <Icon name="zap" :size="13" /> {{ t('overview.eyebrow') }}
          </span>
          <h2>{{ t('overview.headline') }}</h2>
          <p>{{ t('overview.description') }}</p>
          <div class="banner-actions">
            <n-button type="primary" @click="router.push({ name: 'convert' })">
              <template #icon><Icon name="convert" :size="16" /></template>
              {{ t('overview.openConverter') }}
            </n-button>
            <n-button class="ghost" @click="router.push({ name: 'subs' })">
              <template #icon><Icon name="layers" :size="16" /></template>
              {{ t('overview.manageSources') }}
            </n-button>
          </div>
        </div>
        <div class="banner-badges">
          <div class="banner-badge">
            <span class="value">{{ platforms.length }}</span>
            <span class="label-text">{{ t('overview.badgeFormats') }}</span>
          </div>
          <div class="banner-badge">
            <span class="value">{{ protocols.length }}</span>
            <span class="label-text">{{ t('overview.badgeProtocols') }}</span>
          </div>
        </div>
      </section>

      <div class="stat-grid">
        <article class="stat">
          <span class="stat-icon"><Icon name="layers" :size="18" /></span>
          <div class="stat-copy">
            <span class="stat-value">{{ subscriptions.length }}</span>
            <span class="stat-label">{{ t('overview.statSubscriptions') }}</span>
          </div>
          <span class="stat-note">{{ t('overview.statEnabled', { count: activeSubs }) }}</span>
        </article>
        <article class="stat">
          <span class="stat-icon"><Icon name="globe" :size="18" /></span>
          <div class="stat-copy">
            <span class="stat-value">{{ platforms.length }}</span>
            <span class="stat-label">{{ t('overview.statTargets') }}</span>
          </div>
          <span class="stat-note">
            {{ t('overview.statFamilies', { count: groupedPlatforms.length }) }}
          </span>
        </article>
        <article class="stat">
          <span class="stat-icon"><Icon name="server" :size="18" /></span>
          <div class="stat-copy">
            <span class="stat-value">{{ protocols.length }}</span>
            <span class="stat-label">{{ t('overview.statProtocols') }}</span>
          </div>
          <span class="stat-note">{{ t('overview.statUnifiedModel') }}</span>
        </article>
        <article class="stat">
          <span class="stat-icon"><Icon name="shield" :size="18" /></span>
          <div class="stat-copy">
            <span class="stat-value">{{ t('overview.statSsrf') }}</span>
            <span class="stat-label">{{ t('overview.statFetchGuard') }}</span>
          </div>
          <span class="stat-note">{{ t('overview.statPrivateBlocked') }}</span>
        </article>
      </div>

      <section class="card">
        <div class="card-head">
          <span class="title"><Icon name="globe" :size="16" /> {{ t('overview.supportedTargets') }}</span>
          <span class="hint">{{ t('overview.formatCount', { count: platforms.length }) }}</span>
        </div>
        <div class="card-body">
          <div v-for="group in groupedPlatforms" :key="group.group" class="platform-group">
            <span class="platform-group-title">
              {{ t(`platformGroup.${group.group}`) }}
            </span>
            <div class="platform-chips">
              <span v-for="p in group.items" :key="p.id" class="tag">
                {{ p.name }}<em>.{{ p.extension }}</em>
              </span>
            </div>
          </div>
        </div>
      </section>

      <section class="card">
        <div class="card-head">
          <span class="title"><Icon name="layers" :size="16" /> {{ t('overview.recentSources') }}</span>
          <n-button size="tiny" quaternary @click="router.push({ name: 'subs' })">
            {{ t('common.manage') }}
          </n-button>
        </div>
        <div class="card-body">
          <div v-if="recent.length" class="recent-list">
            <article
              v-for="sub in recent"
              :key="sub.id"
              class="sub-card"
              @click="router.push({ name: 'convert', query: { sub: sub.id } })"
            >
              <span class="sub-avatar" :class="{ 'is-local': sub.source === 'local' }">
                {{ (sub.displayName || sub.name).slice(0, 2).toUpperCase() }}
              </span>
              <div class="sub-main">
                <div class="sub-head">
                  <h3 class="sub-title">
                    <span class="name">{{ sub.displayName || sub.name }}</span>
                    <span class="tag primary">{{ sub.source }}</span>
                  </h3>
                </div>
                <p class="sub-detail">
                  <span class="row">
                    <Icon :name="sub.source === 'remote' ? 'cloud' : 'file'" :size="12" />
                    <span class="truncate">{{ sub.url || t('source.inlineContent') }}</span>
                  </span>
                </p>
              </div>
            </article>
          </div>
          <div v-else class="empty">
            <span class="empty-icon"><Icon name="layers" :size="22" /></span>
            <h3>{{ t('overview.emptyTitle') }}</h3>
            <p>{{ t('overview.emptyDescription') }}</p>
            <n-button size="small" type="primary" @click="router.push({ name: 'subs' })">
              {{ t('overview.addSubscription') }}
            </n-button>
          </div>
        </div>
      </section>
    </div>
  </n-spin>
</template>

<style scoped>
.banner {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 26px 28px;
  border: 0;
  color: #eef3fb;
  background: linear-gradient(135deg, #1e3a8a 0%, #2563eb 45%, #478ef2 100%);
  overflow: hidden;
}

.banner::after {
  content: '';
  position: absolute;
  inset: -60% -10% auto auto;
  width: 340px;
  height: 340px;
  background: radial-gradient(circle, rgba(255, 255, 255, 0.22), transparent 62%);
  pointer-events: none;
}

.banner-copy {
  position: relative;
  z-index: 1;
  max-width: 60ch;
}

.banner-eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 11px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.16);
  font-size: 11.5px;
  font-weight: 600;
}

.banner h2 {
  margin: 12px 0 8px;
  font-size: 24px;
  line-height: 1.25;
  color: #fff;
}

.banner p {
  font-size: 13.5px;
  max-width: 52ch;
  opacity: 0.86;
}

.banner-actions {
  display: flex;
  gap: 10px;
  margin-top: 18px;
}

.banner-actions :deep(.ghost) {
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.24);
  color: #fff;
}

.banner-actions :deep(.ghost:hover) {
  background: rgba(255, 255, 255, 0.2);
}

.banner-badges {
  position: relative;
  z-index: 1;
  display: flex;
  gap: 12px;
}

.banner-badge {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 14px 18px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.14);
  border: 1px solid rgba(255, 255, 255, 0.16);
  backdrop-filter: blur(6px);
}

.banner-badge .value {
  font-size: 24px;
  font-weight: 700;
  color: #fff;
  line-height: 1.1;
}

.banner-badge .label-text {
  font-size: 11.5px;
  opacity: 0.82;
}

.platform-group + .platform-group {
  margin-top: 14px;
}

.platform-group-title {
  display: block;
  margin-bottom: 8px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--lowest-text-color);
}

.platform-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}

.platform-chips em {
  font-style: normal;
  opacity: 0.65;
}

.recent-list .sub-card {
  margin-bottom: 10px;
}

.recent-list .sub-card:last-child {
  margin-bottom: 0;
}

@media (max-width: 640px) {
  .banner {
    flex-direction: column;
    align-items: flex-start;
    padding: 22px;
  }

  .banner h2 {
    font-size: 20px;
  }
}
</style>
