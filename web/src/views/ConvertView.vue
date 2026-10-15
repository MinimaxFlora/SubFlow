<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { NButton, NInput, NSelect, NSwitch, useMessage, type SelectOption } from 'naive-ui'
import Icon from '@/components/Icon.vue'
import {
  api,
  type ConvertResult,
  type FetchResult,
  type Operation,
  type OperationType,
  type Platform,
  type Subscription,
} from '@/api/client'
import { translateError, useI18n } from '@/i18n'

const route = useRoute()
const message = useMessage()
const { t } = useI18n()

type SourceMode = 'url' | 'content' | 'saved'

interface ArgField {
  key: string
  label: string
  kind: 'text' | 'select'
  placeholder?: string
  options?: { label: string; value: string }[]
  defaultValue?: string
}

// Field definitions keep i18n keys; labels are resolved for the active locale.
interface FieldDef {
  key: string
  labelKey: string
  kind: 'text' | 'select'
  placeholder?: string
  placeholderKey?: string
  options?: { labelKey: string; value: string }[]
  defaultValue?: string
}

const opFieldDefs: Record<OperationType, FieldDef[]> = {
  Filter: [
    { key: 'regex', labelKey: 'op.pattern', kind: 'text', placeholder: 'HK|Japan|expire' },
    {
      key: 'mode',
      labelKey: 'op.mode',
      kind: 'select',
      defaultValue: 'include',
      options: [
        { labelKey: 'op.mode.include', value: 'include' },
        { labelKey: 'op.mode.exclude', value: 'exclude' },
      ],
    },
    {
      key: 'field',
      labelKey: 'op.field',
      kind: 'select',
      defaultValue: 'name',
      options: [
        { labelKey: 'op.opt.name', value: 'name' },
        { labelKey: 'op.opt.server', value: 'server' },
        { labelKey: 'op.opt.type', value: 'type' },
      ],
    },
  ],
  Rename: [
    { key: 'prefix', labelKey: 'op.prefix', kind: 'text', placeholder: 'SF-' },
    { key: 'suffix', labelKey: 'op.suffix', kind: 'text', placeholder: ' (01)' },
    { key: 'regex', labelKey: 'op.regex', kind: 'text', placeholder: '\\s+node$' },
    { key: 'replacement', labelKey: 'op.replacement', kind: 'text', placeholder: '' },
  ],
  Deduplicate: [
    {
      key: 'by',
      labelKey: 'op.matchBy',
      kind: 'select',
      defaultValue: 'server',
      options: [
        { labelKey: 'op.matchBy.server', value: 'server' },
        { labelKey: 'op.matchBy.full', value: 'full' },
        { labelKey: 'op.opt.name', value: 'name' },
      ],
    },
  ],
  Sort: [
    {
      key: 'by',
      labelKey: 'op.sortBy',
      kind: 'select',
      defaultValue: 'name',
      options: [
        { labelKey: 'op.opt.name', value: 'name' },
        { labelKey: 'op.opt.server', value: 'server' },
        { labelKey: 'op.opt.type', value: 'type' },
        { labelKey: 'op.opt.port', value: 'port' },
      ],
    },
    {
      key: 'order',
      labelKey: 'op.order',
      kind: 'select',
      defaultValue: 'asc',
      options: [
        { labelKey: 'op.order.asc', value: 'asc' },
        { labelKey: 'op.order.desc', value: 'desc' },
      ],
    },
  ],
  UselessFilter: [],
  SetProperty: [
    {
      key: 'key',
      labelKey: 'op.property',
      kind: 'select',
      defaultValue: 'name',
      options: [
        { labelKey: 'op.opt.name', value: 'name' },
        { labelKey: 'op.opt.sni', value: 'sni' },
        { labelKey: 'op.opt.host', value: 'host' },
        { labelKey: 'op.opt.path', value: 'path' },
      ],
    },
    { key: 'value', labelKey: 'op.value', kind: 'text', placeholderKey: 'op.newValue' },
  ],
}

const opTypes = Object.keys(opFieldDefs) as OperationType[]

// Resolve field labels and option labels for the active locale.
const opFields = computed<Record<OperationType, ArgField[]>>(() => {
  const resolved = {} as Record<OperationType, ArgField[]>
  for (const type of opTypes) {
    resolved[type] = opFieldDefs[type].map((field) => ({
      key: field.key,
      label: t(field.labelKey),
      kind: field.kind,
      placeholder: field.placeholderKey ? t(field.placeholderKey) : field.placeholder,
      defaultValue: field.defaultValue,
      options: field.options?.map((option) => ({
        label: t(option.labelKey),
        value: option.value,
      })),
    }))
  }
  return resolved
})

const opTypeOptions = computed(() =>
  opTypes.map((type) => ({ label: t(`op.type.${type}`), value: type })),
)

const uaPresets = computed(() => [
  { label: t('convert.uaDefault'), value: '' },
  { label: 'Clash Verge', value: 'clash-verge/v1.5.0' },
  { label: 'Clash Meta', value: 'mihomo/1.18.0' },
  { label: 'sing-box', value: 'sing-box/1.9.0' },
  { label: 'v2rayN', value: 'v2rayN/6.31' },
  { label: 'Quantumult X', value: 'Quantumult%20X/1.0.30' },
  { label: 'Loon', value: 'Loon/3.1.3' },
  { label: 'Surge', value: 'Surge/5.8.0' },
])

interface EditableOperation {
  type: OperationType
  args: Record<string, string>
}

const sample = [
  'vless://11111111-2222-3333-4444-555555555555@edge.example.com:443?type=ws&security=tls&sni=edge.example.com&path=%2Fws#HK-01',
  'trojan://secret@trojan.example.com:443?sni=trojan.example.com#Japan-02',
  'ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@ss.example.com:8388#US-03',
].join('\n')

const mode = ref<SourceMode>('url')
const url = ref('')
const userAgent = ref('')
const content = ref('')
const savedId = ref<string | null>(null)
const subscriptions = ref<Subscription[]>([])
const platform = ref('clashmeta')
const includeUnsupported = ref(false)
const operations = reactive<EditableOperation[]>([])
const result = ref<ConvertResult | null>(null)
const fetchInfo = ref<FetchResult | null>(null)
const running = ref(false)
const fetching = ref(false)
const platformOptions = ref<SelectOption[]>([])
const platformMap = ref<Record<string, Platform>>({})

const modeOptions = computed(() => [
  { label: t('convert.modeUrl'), value: 'url' },
  { label: t('convert.modeContent'), value: 'content' },
  { label: t('convert.modeSaved'), value: 'saved' },
])

const savedOptions = computed(() =>
  subscriptions.value.map((s) => ({ label: s.displayName || s.name, value: s.id })),
)

const selectedPlatform = computed(() => platformMap.value[platform.value])

const canRun = computed(() => {
  if (mode.value === 'url') return url.value.trim() !== ''
  if (mode.value === 'content') return content.value.trim() !== ''
  return savedId.value !== null
})

const outputLines = computed(() => {
  if (!result.value?.output) return 0
  return result.value.output.split('\n').length
})

onMounted(async () => {
  try {
    const platforms = await api.listPlatforms()
    const map: Record<string, Platform> = {}
    for (const p of platforms) map[p.id] = p
    platformMap.value = map

    const groups = new Map<string, Platform[]>()
    for (const p of platforms) {
      const list = groups.get(p.group) ?? []
      list.push(p)
      groups.set(p.group, list)
    }
    platformOptions.value = [...groups.entries()].map(([group, items]) => ({
      type: 'group',
      // Prefix the key so a group never collides with an option value.
      key: `group:${group}`,
      label: group.toUpperCase(),
      children: items
        .slice()
        .sort((a, b) => a.name.localeCompare(b.name))
        .map((p) => ({ label: `${p.name} (.${p.extension})`, value: p.id })),
    }))
  } catch (err) {
    message.error(translateError(err))
  }

  try {
    subscriptions.value = await api.listSubscriptions()
  } catch {
    subscriptions.value = []
  }

  const preset = route.query.sub as string | undefined
  if (preset) {
    mode.value = 'saved'
    savedId.value = preset
  }
})

function defaultArgs(type: OperationType) {
  const args: Record<string, string> = {}
  for (const field of opFieldDefs[type]) {
    if (field.defaultValue !== undefined) args[field.key] = field.defaultValue
  }
  return args
}

function addOperation() {
  const type: OperationType = 'Filter'
  operations.push({ type, args: defaultArgs(type) })
}

function changeType(op: EditableOperation, type: OperationType) {
  op.type = type
  op.args = defaultArgs(type)
}

function removeOperation(index: number) {
  operations.splice(index, 1)
}

function loadSample() {
  mode.value = 'content'
  content.value = sample
}

async function testFetch() {
  if (!url.value.trim()) {
    message.warning(t('convert.enterUrl'))
    return
  }
  fetching.value = true
  fetchInfo.value = null
  try {
    const info = await api.fetchSubscription({ url: url.value.trim(), userAgent: userAgent.value })
    fetchInfo.value = info
    if (info.nodes > 0) {
      message.success(t('convert.fetched', { count: info.nodes }))
    } else {
      message.warning(info.parseError || t('convert.noNodes'))
    }
  } catch (err) {
    message.error(translateError(err))
  } finally {
    fetching.value = false
  }
}

async function run() {
  running.value = true
  fetchInfo.value = null
  try {
    const payload: Operation[] = operations.map((op) => ({
      type: op.type,
      args: Object.fromEntries(Object.entries(op.args).filter(([, v]) => v !== '')),
    }))
    const input = {
      content: '',
      url: '',
      userAgent: userAgent.value,
      platform: platform.value,
      operations: payload,
      includeUnsupported: includeUnsupported.value,
    }
    if (mode.value === 'url') {
      input.url = url.value.trim()
    } else if (mode.value === 'saved') {
      const resolved = await api.subscriptionContent(savedId.value as string)
      input.content = resolved.content
    } else {
      input.content = content.value
    }
    result.value = await api.convert(input)
    message.success(
      t('convert.emittedMsg', { emitted: result.value.emitted, total: result.value.total }),
    )
  } catch (err) {
    result.value = null
    message.error(translateError(err))
  } finally {
    running.value = false
  }
}

async function copyOutput() {
  if (!result.value) return
  try {
    await navigator.clipboard.writeText(result.value.output)
    message.success(t('convert.copied'))
  } catch {
    message.error(t('common.clipboard'))
  }
}

function downloadOutput() {
  if (!result.value) return
  const ext = selectedPlatform.value?.extension ?? 'txt'
  const blob = new Blob([result.value.output], { type: 'text/plain' })
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = `subflow-${platform.value}.${ext}`
  link.click()
  URL.revokeObjectURL(link.href)
}
</script>

<template>
  <div class="views">
    <section class="card">
      <div class="card-head">
        <span class="title"><Icon name="link" :size="16" /> {{ t('convert.source') }}</span>
        <n-button size="tiny" quaternary @click="loadSample">
          <template #icon><Icon name="zap" :size="14" /></template>
          {{ t('convert.sample') }}
        </n-button>
      </div>
      <div class="card-body source-body">
        <div class="segmented">
          <button
            v-for="opt in modeOptions"
            :key="opt.value"
            type="button"
            class="segment"
            :class="{ active: mode === opt.value }"
            @click="mode = opt.value as SourceMode"
          >
            {{ opt.label }}
          </button>
        </div>

        <div v-if="mode === 'url'" class="source-block">
          <n-input v-model:value="url" placeholder="https://example.com/api/v1/subscribe?token=...">
            <template #prefix><Icon name="link" :size="15" /></template>
          </n-input>
          <div class="row wrap">
            <n-select v-model:value="userAgent" :options="uaPresets" size="small" style="max-width: 220px" />
            <n-button size="small" :loading="fetching" @click="testFetch">
              <template #icon><Icon name="cloud" :size="14" /></template>
              {{ t('convert.testFetch') }}
            </n-button>
          </div>
          <div v-if="fetchInfo" class="note" :class="fetchInfo.nodes ? 'ok' : 'bad'">
            <Icon :name="fetchInfo.nodes ? 'check' : 'alert'" :size="15" />
            <span v-if="fetchInfo.nodes">
              {{
                t('convert.fetchNote', {
                  count: fetchInfo.nodes,
                  size: (fetchInfo.bytes / 1024).toFixed(1),
                  url: fetchInfo.finalUrl,
                })
              }}
            </span>
            <span v-else>{{ fetchInfo.parseError || t('convert.noNodes') }}</span>
          </div>
        </div>

        <div v-else-if="mode === 'content'" class="source-block">
          <n-input
            v-model:value="content"
            type="textarea"
            :autosize="{ minRows: 6, maxRows: 14 }"
            :placeholder="t('convert.contentPlaceholder')"
          />
        </div>

        <div v-else class="source-block">
          <n-select
            v-model:value="savedId"
            :options="savedOptions"
            :placeholder="t('convert.chooseSubscription')"
          />
          <p v-if="!savedOptions.length" class="hint">{{ t('convert.noSaved') }}</p>
        </div>
      </div>
    </section>

    <section class="card">
      <div class="card-head">
        <span class="title"><Icon name="globe" :size="16" /> {{ t('convert.target') }}</span>
        <span v-if="selectedPlatform" class="tag primary">
          {{ selectedPlatform.name }} · .{{ selectedPlatform.extension }}
        </span>
      </div>
      <div class="card-body source-body">
        <n-select v-model:value="platform" :options="platformOptions" filterable />
        <label class="toggle">
          <n-switch v-model:value="includeUnsupported" size="small" />
          <span>{{ t('convert.keepUnsupported') }}</span>
        </label>
      </div>
    </section>

    <section class="card">
      <div class="card-head">
        <span class="title">
          <Icon name="sliders" :size="16" /> {{ t('convert.operations') }}
        </span>
        <n-button size="tiny" @click="addOperation">
          <template #icon><Icon name="plus" :size="14" /></template>
          {{ t('common.add') }}
        </n-button>
      </div>
      <div class="card-body">
        <div v-if="operations.length" class="ops">
          <div v-for="(op, index) in operations" :key="index" class="op">
            <div class="op-head">
              <span class="op-index">{{ index + 1 }}</span>
              <n-select
                :value="op.type"
                :options="opTypeOptions"
                size="small"
                style="width: 150px"
                @update:value="(value: OperationType) => changeType(op, value)"
              />
              <button class="icon-btn danger" :title="t('convert.removeOp')" @click="removeOperation(index)">
                <Icon name="x" :size="15" />
              </button>
            </div>
            <div v-if="opFields[op.type].length" class="op-args">
              <div v-for="field in opFields[op.type]" :key="field.key" class="op-arg">
                <span class="arg-label">{{ field.label }}</span>
                <n-select
                  v-if="field.kind === 'select'"
                  v-model:value="op.args[field.key]"
                  :options="field.options"
                  size="small"
                />
                <n-input
                  v-else
                  v-model:value="op.args[field.key]"
                  size="small"
                  :placeholder="field.placeholder"
                />
              </div>
            </div>
            <p v-else class="hint">{{ t('convert.noParams') }}</p>
          </div>
        </div>
        <p v-else class="hint">{{ t('convert.opsHint') }}</p>
      </div>
    </section>

    <n-button type="primary" block size="large" :disabled="!canRun" :loading="running" @click="run">
      <template #icon><Icon name="play" :size="17" /></template>
      {{ t('convert.run') }}
    </n-button>

    <section class="card">
      <div class="card-head">
        <span class="title"><Icon name="server" :size="16" /> {{ t('convert.output') }}</span>
        <div class="row">
          <span v-if="result" class="tag ok">{{ result.platform }}</span>
          <n-button v-if="result" size="tiny" @click="copyOutput">
            <template #icon><Icon name="copy" :size="14" /></template>
            {{ t('common.copy') }}
          </n-button>
          <n-button v-if="result" size="tiny" @click="downloadOutput">
            <template #icon><Icon name="download" :size="14" /></template>
            {{ t('common.download') }}
          </n-button>
        </div>
      </div>
      <div class="card-body">
        <template v-if="result">
          <div class="stats-row">
            <div class="stat-mini">
              <span class="stat-mini-value">{{ result.total }}</span>
              <span class="stat-mini-label">{{ t('convert.parsed') }}</span>
            </div>
            <div class="stat-mini">
              <span class="stat-mini-value">{{ result.emitted }}</span>
              <span class="stat-mini-label">{{ t('convert.emitted') }}</span>
            </div>
            <div class="stat-mini">
              <span class="stat-mini-value">{{ outputLines }}</span>
              <span class="stat-mini-label">{{ t('convert.lines') }}</span>
            </div>
          </div>
          <div v-if="result.source" class="note">
            <Icon name="link" :size="15" />
            <span class="truncate">{{ result.source }}</span>
          </div>
          <div v-if="result.warnings?.length" class="note warn">
            <Icon name="alert" :size="15" />
            <div>
              <p v-for="warning in result.warnings" :key="warning">{{ warning }}</p>
            </div>
          </div>
          <pre class="code-panel">{{ result.output || t('convert.emptyOutput') }}</pre>
        </template>
        <div v-else class="empty">
          <span class="empty-icon"><Icon name="convert" :size="22" /></span>
          <h3>{{ t('convert.noOutputTitle') }}</h3>
          <p>{{ t('convert.noOutputHint') }}</p>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.source-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.source-block {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.segmented {
  display: inline-flex;
  align-self: flex-start;
  padding: 3px;
  border-radius: 11px;
  background: var(--divider-color);
}

.segment {
  padding: 7px 14px;
  border: 0;
  border-radius: 9px;
  background: transparent;
  color: var(--comment-text-color);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.14s ease, color 0.14s ease;
}

.segment.active {
  background: var(--card-color);
  color: var(--primary-color);
  box-shadow: 0 1px 2px var(--shadow-color);
}

.toggle {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  font-size: 13px;
  color: var(--second-text-color);
  cursor: pointer;
}

.ops {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.op {
  padding: 12px;
  border-radius: 11px;
  background: var(--divider-color);
}

.op-head {
  display: flex;
  align-items: center;
  gap: 10px;
}

.op-index {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: 7px;
  background: var(--primary-soft);
  color: var(--primary-color);
  font-size: 12px;
  font-weight: 700;
  flex: none;
}

.op-args {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 12px;
}

.op-arg {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.arg-label {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--comment-text-color);
}

.stats-row {
  display: flex;
  gap: 10px;
  margin-bottom: 12px;
}

.stat-mini {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  flex: 1;
  padding: 10px;
  border-radius: 11px;
  background: var(--divider-color);
}

.stat-mini-value {
  font-size: 19px;
  font-weight: 700;
  color: var(--primary-text-color);
}

.stat-mini-label {
  font-size: 11.5px;
  color: var(--comment-text-color);
}

.code-panel {
  margin-top: 12px;
}

@media (max-width: 640px) {
  .op-args {
    grid-template-columns: 1fr;
  }
}
</style>
