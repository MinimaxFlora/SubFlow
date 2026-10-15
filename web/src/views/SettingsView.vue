<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NButton, NInput, NSelect, NSpin, useMessage } from 'naive-ui'
import Icon from '@/components/Icon.vue'
import { api, type VersionInfo } from '@/api/client'
import { setLocale, supportedLocales, translateError, useI18n, type LocaleCode } from '@/i18n'

const message = useMessage()
const { locale, t } = useI18n()
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

const localeOptions = supportedLocales.map((item) => ({ label: item.label, value: item.code }))
const selectedLocale = computed({
  get: () => locale.value,
  set: (value: LocaleCode) => setLocale(value),
})

async function reload() {
  loading.value = true
  try {
    const [settings, versionInfo] = await Promise.all([api.listSettings(), api.version()])
    rows.value = Object.entries(settings)
      .map(([key, value]) => ({ key, value }))
      .sort((a, b) => a.key.localeCompare(b.key))
    version.value = versionInfo
  } catch (err) {
    message.error(translateError(err))
  } finally {
    loading.value = false
  }
}

async function saveRow(row: SettingRow) {
  try {
    await api.updateSettings({ [row.key]: row.value })
    message.success(t('settings.msg.saved', { key: row.key }))
  } catch (err) {
    message.error(translateError(err))
  }
}

async function clearRow(row: SettingRow) {
  row.value = ''
  await saveRow(row)
}

async function addSetting() {
  const key = newKey.value.trim()
  if (!key) {
    message.warning(t('settings.msg.keyRequired'))
    return
  }
  saving.value = true
  try {
    await api.updateSettings({ [key]: newValue.value })
    newKey.value = ''
    newValue.value = ''
    await reload()
    message.success(t('settings.msg.settingSaved'))
  } catch (err) {
    message.error(translateError(err))
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
          <span class="title">
            <Icon name="sliders" :size="16" /> {{ t('settings.title') }}
          </span>
          <span class="hint">{{ t('settings.keyCount', { count: rows.length }) }}</span>
        </div>
        <div class="card-body">
          <div v-if="rows.length" class="rows">
            <div v-for="row in rows" :key="row.key" class="setting-row">
              <code class="key">{{ row.key }}</code>
              <n-input
                v-model:value="row.value"
                size="small"
                :placeholder="t('common.emptyValue')"
              />
              <div class="row-actions">
                <n-button size="tiny" @click="saveRow(row)">{{ t('common.save') }}</n-button>
                <n-button size="tiny" quaternary @click="clearRow(row)">
                  {{ t('common.clear') }}
                </n-button>
              </div>
            </div>
          </div>
          <div v-else class="empty compact">
            <span class="empty-icon"><Icon name="sliders" :size="22" /></span>
            <h3>{{ t('settings.emptyTitle') }}</h3>
            <p>{{ t('settings.emptyDescription') }}</p>
          </div>

          <div class="add-row">
            <n-input v-model:value="newKey" size="small" :placeholder="t('settings.keyPlaceholder')" />
            <n-input
              v-model:value="newValue"
              size="small"
              :placeholder="t('settings.valuePlaceholder')"
            />
            <n-button size="small" type="primary" :loading="saving" @click="addSetting">
              {{ t('common.add') }}
            </n-button>
          </div>
        </div>
      </section>

      <section class="card">
        <div class="card-head">
          <span class="title"><Icon name="info" :size="16" /> {{ t('settings.about') }}</span>
        </div>
        <div class="card-body">
          <div class="about-row">
            <span class="about-label">{{ t('settings.language') }}</span>
            <n-select
              v-model:value="selectedLocale"
              :options="localeOptions"
              size="small"
              style="width: 160px"
            />
          </div>
          <div class="about-row">
            <span class="about-label">{{ t('settings.version') }}</span>
            <code>{{ version?.version || '-' }}</code>
          </div>
          <div class="about-row">
            <span class="about-label">{{ t('settings.commit') }}</span>
            <code>{{ version?.commit || '-' }}</code>
          </div>
          <div class="about-row">
            <span class="about-label">{{ t('settings.built') }}</span>
            <code>{{ version?.buildTime || '-' }}</code>
          </div>
          <div class="note about-note">
            <Icon name="info" :size="15" />
            <span>{{ t('settings.aboutNote') }}</span>
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
