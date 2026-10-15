<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton, NInput, NSpin, useMessage } from 'naive-ui'
import Icon from '@/components/Icon.vue'
import { api, type VersionInfo } from '@/api/client'

const message = useMessage()
const loading = ref(true)
const saving = ref(false)
const version = ref<VersionInfo | null>(null)

interface SettingRow {
  key: string
  value: string
}

const rows = ref<SettingRow[]>([])
const newKey = ref('')
const newValue = ref('')

async function reload() {
  loading.value = true
  try {
    const [settings, versionInfo] = await Promise.all([api.listSettings(), api.version()])
    rows.value = Object.entries(settings)
      .map(([key, value]) => ({ key, value }))
      .sort((a, b) => a.key.localeCompare(b.key))
    version.value = versionInfo
  } catch (err) {
    message.error(err instanceof Error ? err.message : String(err))
  } finally {
    loading.value = false
  }
}

async function saveRow(row: SettingRow) {
  try {
    await api.updateSettings({ [row.key]: row.value })
    message.success(`Saved ${row.key}`)
  } catch (err) {
    message.error(err instanceof Error ? err.message : String(err))
  }
}

async function clearRow(row: SettingRow) {
  row.value = ''
  await saveRow(row)
}

async function addSetting() {
  const key = newKey.value.trim()
  if (!key) {
    message.warning('Key is required')
    return
  }
  saving.value = true
  try {
    await api.updateSettings({ [key]: newValue.value })
    newKey.value = ''
    newValue.value = ''
    await reload()
    message.success('Setting saved')
  } catch (err) {
    message.error(err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

onMounted(reload)
</script>

<template>
  <div class="views">
    <n-spin :show="loading">
      <section class="card">
        <div class="card-head">
          <span class="title"><Icon name="sliders" :size="16" /> Runtime settings</span>
          <span class="hint">{{ rows.length }} key(s)</span>
        </div>
        <div class="card-body">
          <div v-if="rows.length" class="rows">
            <div v-for="row in rows" :key="row.key" class="setting-row">
              <code class="key">{{ row.key }}</code>
              <n-input v-model:value="row.value" size="small" placeholder="(empty)" />
              <div class="row-actions">
                <n-button size="tiny" @click="saveRow(row)">Save</n-button>
                <n-button size="tiny" quaternary @click="clearRow(row)">Clear</n-button>
              </div>
            </div>
          </div>
          <div v-else class="empty compact">
            <span class="empty-icon"><Icon name="sliders" :size="22" /></span>
            <h3>No settings stored</h3>
            <p>Key and value pairs are persisted next to your subscriptions and read by the backend at runtime.</p>
          </div>

          <div class="add-row">
            <n-input v-model:value="newKey" size="small" placeholder="key" />
            <n-input v-model:value="newValue" size="small" placeholder="value" />
            <n-button size="small" type="primary" :loading="saving" @click="addSetting">Add</n-button>
          </div>
        </div>
      </section>

      <section class="card">
        <div class="card-head">
          <span class="title"><Icon name="info" :size="16" /> About</span>
        </div>
        <div class="card-body">
          <div class="about-row">
            <span class="about-label">Version</span>
            <code>{{ version?.version || '-' }}</code>
          </div>
          <div class="about-row">
            <span class="about-label">Commit</span>
            <code>{{ version?.commit || '-' }}</code>
          </div>
          <div class="about-row">
            <span class="about-label">Built</span>
            <code>{{ version?.buildTime || '-' }}</code>
          </div>
          <div class="note about-note">
            <Icon name="info" :size="15" />
            <span>
              SubFlow keeps everything in a single SQLite database and embeds the compiled frontend, so
              one binary is enough to run it.
            </span>
          </div>
        </div>
      </section>
    </n-spin>
  </div>
</template>

<style scoped>
.rows {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.setting-row {
  display: grid;
  grid-template-columns: minmax(110px, 180px) minmax(0, 1fr) auto;
  gap: 10px;
  align-items: center;
}

.key {
  font-size: 12.5px;
  overflow-wrap: anywhere;
}

.row-actions {
  display: flex;
  gap: 6px;
}

.add-row {
  display: grid;
  grid-template-columns: 1fr 1fr auto;
  gap: 10px;
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px dashed var(--divider-color);
}

.empty.compact {
  padding: 32px 20px;
}

.about-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 8px 0;
}

.about-row + .about-row {
  border-top: 1px solid var(--divider-color);
}

.about-label {
  color: var(--comment-text-color);
  font-size: 13px;
}

.about-note {
  margin-top: 14px;
  line-height: 1.6;
}

@media (max-width: 640px) {
  .setting-row,
  .add-row {
    grid-template-columns: 1fr;
  }
}
</style>
