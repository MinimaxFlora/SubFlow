<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  NButton,
  NDrawer,
  NDrawerContent,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSpin,
  NSwitch,
  useDialog,
  useMessage,
} from 'naive-ui'
import Icon from '@/components/Icon.vue'
import SubCard from '@/components/SubCard.vue'
import { api, type Subscription, type SubscriptionInput } from '@/api/client'
import { translateError, useI18n } from '@/i18n'

const router = useRouter()
const message = useMessage()
const dialog = useDialog()
const { t } = useI18n()

const loading = ref(false)
const saving = ref(false)
const subscriptions = ref<Subscription[]>([])
const search = ref('')
const sourceFilter = ref<'all' | 'remote' | 'local'>('all')

const drawerOpen = ref(false)
const editingId = ref<string | null>(null)
const form = reactive<SubscriptionInput & { content?: string }>({
  name: '',
  displayName: '',
  source: 'remote',
  url: '',
  content: '',
  enabled: true,
  sort: 0,
})

const filters = computed<{ key: 'all' | 'remote' | 'local'; label: string }[]>(() => [
  { key: 'all', label: t('source.all') },
  { key: 'remote', label: t('source.remote') },
  { key: 'local', label: t('source.local') },
])

const filtered = computed(() =>
  subscriptions.value.filter((sub) => {
    if (sourceFilter.value !== 'all' && sub.source !== sourceFilter.value) return false
    const q = search.value.trim().toLowerCase()
    if (!q) return true
    return (
      sub.name.toLowerCase().includes(q) ||
      sub.displayName.toLowerCase().includes(q) ||
      sub.url.toLowerCase().includes(q)
    )
  }),
)

async function reload() {
  loading.value = true
  try {
    subscriptions.value = await api.listSubscriptions()
  } catch (err) {
    message.error(translateError(err))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = null
  Object.assign(form, {
    name: '',
    displayName: '',
    source: 'remote',
    url: '',
    content: '',
    enabled: true,
    sort: subscriptions.value.length,
  })
  drawerOpen.value = true
}

function openEdit(sub: Subscription) {
  editingId.value = sub.id
  Object.assign(form, {
    name: sub.name,
    displayName: sub.displayName,
    source: sub.source,
    url: sub.url,
    content: '',
    enabled: sub.enabled,
    sort: sub.sort,
  })
  drawerOpen.value = true
}

async function submit() {
  if (!form.name.trim()) {
    message.warning(t('subs.msg.nameRequired'))
    return
  }
  if (form.source === 'remote' && !form.url.trim()) {
    message.warning(t('subs.msg.urlRequired'))
    return
  }
  if (form.source === 'local' && !editingId.value && !form.content?.trim()) {
    message.warning(t('subs.msg.contentRequired'))
    return
  }
  saving.value = true
  try {
    const payload: SubscriptionInput = {
      name: form.name.trim(),
      displayName: form.displayName.trim(),
      source: form.source,
      url: form.url.trim(),
      enabled: form.enabled,
      sort: form.sort,
    }
    if (form.content?.trim()) payload.content = form.content
    if (editingId.value) {
      await api.updateSubscription(editingId.value, payload)
      message.success(t('subs.msg.updated'))
    } else {
      await api.createSubscription(payload)
      message.success(t('subs.msg.created'))
    }
    drawerOpen.value = false
    await reload()
  } catch (err) {
    message.error(translateError(err))
  } finally {
    saving.value = false
  }
}

async function toggle(sub: Subscription) {
  try {
    await api.updateSubscription(sub.id, {
      name: sub.name,
      displayName: sub.displayName,
      source: sub.source,
      url: sub.url,
      enabled: !sub.enabled,
      sort: sub.sort,
    })
    await reload()
  } catch (err) {
    message.error(translateError(err))
  }
}

function confirmRemove(sub: Subscription) {
  dialog.warning({
    title: t('subs.dialogTitle'),
    content: t('subs.dialogContent', { name: sub.displayName || sub.name }),
    positiveText: t('common.delete'),
    negativeText: t('common.cancel'),
    onPositiveClick: async () => {
      try {
        await api.deleteSubscription(sub.id)
        message.success(t('subs.msg.deleted'))
        await reload()
      } catch (err) {
        message.error(translateError(err))
      }
    },
  })
}

function convert(sub: Subscription) {
  router.push({ name: 'convert', query: { sub: sub.id } })
}

async function copyLink(sub: Subscription) {
  const text = sub.url || `${window.location.origin}/api/subscriptions/${sub.id}/content`
  try {
    await navigator.clipboard.writeText(text)
    message.success(t('subs.msg.linkCopied'))
  } catch {
    message.error(t('common.clipboard'))
  }
}

onMounted(reload)
</script>

<template>
  <div>
    <div class="tag-bar">
      <button
        v-for="f in filters"
        :key="f.key"
        type="button"
        class="tag-chip"
        :class="{ current: sourceFilter === f.key }"
        @click="sourceFilter = f.key"
      >
        {{ f.label }}
      </button>
      <n-input
        v-model:value="search"
        :placeholder="t('subs.searchPlaceholder')"
        clearable
        size="small"
        style="flex: 1; min-width: 110px; margin-left: 8px"
      >
        <template #prefix><Icon name="search" :size="14" /></template>
      </n-input>
    </div>

    <div class="section-title">
      <span>{{ t('subs.title') }}</span>
      <span class="count">({{ filtered.length }}/{{ subscriptions.length }})</span>
    </div>

    <n-spin :show="loading">
      <div v-if="filtered.length" class="sub-list">
        <SubCard
          v-for="sub in filtered"
          :key="sub.id"
          :sub="sub"
          @open="convert"
          @edit="openEdit"
          @remove="confirmRemove"
          @toggle="toggle"
          @copy="copyLink"
        />
      </div>

      <div v-else class="empty">
        <span class="empty-icon"><Icon name="layers" :size="30" /></span>
        <h3>
          {{
            subscriptions.length
              ? t('subs.emptyMatchTitle')
              : t('subs.emptyTitle')
          }}
        </h3>
        <p>
          {{
            subscriptions.length
              ? t('subs.emptyMatchDescription')
              : t('subs.emptyDescription')
          }}
        </p>
        <n-button v-if="!subscriptions.length" type="primary" @click="openCreate">
          <template #icon><Icon name="plus" :size="15" /></template>
          {{ t('subs.add') }}
        </n-button>
      </div>
    </n-spin>

    <div class="fab-stack">
      <button type="button" class="fab refresh" :title="t('common.refresh')" @click="reload">
        <Icon name="refresh" :size="20" />
      </button>
      <button type="button" class="fab" :title="t('subs.add')" @click="openCreate">
        <Icon name="plus" :size="22" />
      </button>
    </div>

    <n-drawer v-model:show="drawerOpen" placement="bottom" :height="'auto'" :auto-focus="false">
      <n-drawer-content
        :title="editingId ? t('subs.drawerEdit') : t('subs.drawerNew')"
        closable
      >
        <n-form label-placement="top">
          <n-form-item :label="t('subs.form.name')" required>
            <n-input v-model:value="form.name" :placeholder="t('subs.form.namePlaceholder')" />
          </n-form-item>
          <n-form-item :label="t('subs.form.displayName')">
            <n-input
              v-model:value="form.displayName"
              :placeholder="t('subs.form.displayNamePlaceholder')"
            />
          </n-form-item>
          <n-form-item :label="t('subs.form.sourceType')">
            <div class="source-switch">
              <button
                type="button"
                class="tag-chip"
                :class="{ current: form.source === 'remote' }"
                @click="form.source = 'remote'"
              >
                <Icon name="cloud" :size="14" /> {{ t('source.remoteUrl') }}
              </button>
              <button
                type="button"
                class="tag-chip"
                :class="{ current: form.source === 'local' }"
                @click="form.source = 'local'"
              >
                <Icon name="file" :size="14" /> {{ t('source.localContent') }}
              </button>
            </div>
          </n-form-item>
          <n-form-item v-if="form.source === 'remote'" :label="t('subs.form.url')" required>
            <n-input
              v-model:value="form.url"
              :placeholder="t('subs.form.urlPlaceholder')"
            />
          </n-form-item>
          <n-form-item
            v-else
            :label="editingId ? t('subs.form.replaceContent') : t('subs.form.content')"
            required
          >
            <n-input
              v-model:value="form.content"
              type="textarea"
              :autosize="{ minRows: 5, maxRows: 12 }"
              :placeholder="t('subs.form.contentPlaceholder')"
            />
          </n-form-item>
          <div class="form-grid">
            <n-form-item :label="t('subs.form.sort')">
              <n-input-number v-model:value="form.sort" :min="0" style="width: 100%" />
            </n-form-item>
            <n-form-item :label="t('common.enabled')">
              <n-switch v-model:value="form.enabled" />
            </n-form-item>
          </div>
        </n-form>
        <template #footer>
          <div class="modal-footer">
            <n-button @click="drawerOpen = false">{{ t('common.cancel') }}</n-button>
            <n-button type="primary" :loading="saving" @click="submit">
              {{ t('common.save') }}
            </n-button>
          </div>
        </template>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.source-switch {
  display: flex;
  gap: 8px;
}

.source-switch .tag-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 10px;
  background: var(--divider-color);
  color: var(--comment-text-color);
}

.source-switch .tag-chip.current {
  border-bottom-color: transparent;
  background: var(--primary-soft);
  color: var(--primary-color);
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
