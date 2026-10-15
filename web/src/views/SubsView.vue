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

const router = useRouter()
const message = useMessage()
const dialog = useDialog()

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

const filters: { key: 'all' | 'remote' | 'local'; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'remote', label: 'Remote' },
  { key: 'local', label: 'Local' },
]

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
    message.error(err instanceof Error ? err.message : String(err))
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
    message.warning('Name is required')
    return
  }
  if (form.source === 'remote' && !form.url.trim()) {
    message.warning('URL is required for remote sources')
    return
  }
  if (form.source === 'local' && !editingId.value && !form.content?.trim()) {
    message.warning('Content is required for local sources')
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
      message.success('Subscription updated')
    } else {
      await api.createSubscription(payload)
      message.success('Subscription created')
    }
    drawerOpen.value = false
    await reload()
  } catch (err) {
    message.error(err instanceof Error ? err.message : String(err))
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
    message.error(err instanceof Error ? err.message : String(err))
  }
}

function confirmRemove(sub: Subscription) {
  dialog.warning({
    title: 'Delete subscription',
    content: `Remove "${sub.displayName || sub.name}"?`,
    positiveText: 'Delete',
    negativeText: 'Cancel',
    onPositiveClick: async () => {
      try {
        await api.deleteSubscription(sub.id)
        message.success('Subscription deleted')
        await reload()
      } catch (err) {
        message.error(err instanceof Error ? err.message : String(err))
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
    message.success('Link copied')
  } catch {
    message.error('Clipboard unavailable')
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
        placeholder="Search"
        clearable
        size="small"
        style="flex: 1; min-width: 110px; margin-left: 8px"
      >
        <template #prefix><Icon name="search" :size="14" /></template>
      </n-input>
    </div>

    <div class="section-title">
      <span>Subscriptions</span>
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
        <h3>{{ subscriptions.length ? 'No matching sources' : 'No subscriptions yet' }}</h3>
        <p>
          {{
            subscriptions.length
              ? 'Adjust your search or filter to find a source.'
              : 'Add a remote URL or paste local content, then convert it to any client format.'
          }}
        </p>
        <n-button v-if="!subscriptions.length" type="primary" @click="openCreate">
          <template #icon><Icon name="plus" :size="15" /></template>
          Add subscription
        </n-button>
      </div>
    </n-spin>

    <div class="fab-stack">
      <button type="button" class="fab refresh" title="Refresh" @click="reload">
        <Icon name="refresh" :size="20" />
      </button>
      <button type="button" class="fab" title="Add subscription" @click="openCreate">
        <Icon name="plus" :size="22" />
      </button>
    </div>

    <n-drawer v-model:show="drawerOpen" placement="bottom" :height="'auto'" :auto-focus="false">
      <n-drawer-content
        :title="editingId ? 'Edit subscription' : 'New subscription'"
        closable
      >
        <n-form label-placement="top">
          <n-form-item label="Name" required>
            <n-input v-model:value="form.name" placeholder="my-provider" />
          </n-form-item>
          <n-form-item label="Display name">
            <n-input v-model:value="form.displayName" placeholder="Optional label" />
          </n-form-item>
          <n-form-item label="Source type">
            <div class="source-switch">
              <button
                type="button"
                class="tag-chip"
                :class="{ current: form.source === 'remote' }"
                @click="form.source = 'remote'"
              >
                <Icon name="cloud" :size="14" /> Remote URL
              </button>
              <button
                type="button"
                class="tag-chip"
                :class="{ current: form.source === 'local' }"
                @click="form.source = 'local'"
              >
                <Icon name="file" :size="14" /> Local content
              </button>
            </div>
          </n-form-item>
          <n-form-item v-if="form.source === 'remote'" label="Subscription URL" required>
            <n-input
              v-model:value="form.url"
              placeholder="https://example.com/api/v1/subscribe?token=..."
            />
          </n-form-item>
          <n-form-item v-else :label="editingId ? 'Replace content (optional)' : 'Content'" required>
            <n-input
              v-model:value="form.content"
              type="textarea"
              :autosize="{ minRows: 5, maxRows: 12 }"
              placeholder="Paste a URI list or Base64 subscription"
            />
          </n-form-item>
          <div class="form-grid">
            <n-form-item label="Sort weight">
              <n-input-number v-model:value="form.sort" :min="0" style="width: 100%" />
            </n-form-item>
            <n-form-item label="Enabled">
              <n-switch v-model:value="form.enabled" />
            </n-form-item>
          </div>
        </n-form>
        <template #footer>
          <div class="modal-footer">
            <n-button @click="drawerOpen = false">Cancel</n-button>
            <n-button type="primary" :loading="saving" @click="submit">Save</n-button>
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
