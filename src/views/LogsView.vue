<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import {
  LogApiError,
  clearLogToken,
  fetchLogMeta,
  getLogToken,
  normalizeLogId,
  searchLogs,
  setLogToken,
  type LogContextLines,
  type LogEntry,
  type LogEnv,
  type LogMeta,
  type LogSearchResponse,
} from '@/api/logs'

const ENV_OPTIONS: { value: LogEnv; label: string }[] = [
  { value: 'all', label: '全部环境' },
  { value: 'prod', label: '生产' },
  { value: 'test', label: '测试' },
]

const LINES_OPTIONS: { value: LogContextLines; label: string }[] = [
  { value: 30, label: '上下 30 行' },
  { value: 50, label: '上下 50 行' },
  { value: 100, label: '上下 100 行' },
]

const RANGE_OPTIONS = [
  { value: '1h', label: '最近 1 小时' },
  { value: '6h', label: '最近 6 小时' },
  { value: '24h', label: '最近 24 小时' },
  { value: '7d', label: '最近 7 天' },
  { value: '14d', label: '最近 14 天' },
]

const route = useRoute()
const router = useRouter()

function initialEnv(): LogEnv {
  const raw = String(route.query.env ?? '')
  return raw === 'prod' || raw === 'test' ? raw : 'all'
}

function initialLines(): LogContextLines {
  const value = Number(route.query.lines)
  return value === 50 || value === 100 ? value : 30
}

function initialRange(): string {
  const raw = String(route.query.range ?? '')
  return RANGE_OPTIONS.some((option) => option.value === raw) ? raw : '24h'
}

const meta = ref<LogMeta | null>(null)
const metaError = ref('')
const idInput = ref(String(route.query.id ?? ''))
const env = ref<LogEnv>(initialEnv())
const lines = ref<LogContextLines>(initialLines())
const range = ref(initialRange())
const token = ref(getLogToken())
const tokenNeeded = ref(false)
const loading = ref(false)
const error = ref('')
const result = ref<LogSearchResponse | null>(null)
const elapsedMs = ref(0)
const copiedKey = ref('')
const expanded = ref<Set<string>>(new Set())

const showToken = computed(
  () => tokenNeeded.value || Boolean(meta.value?.authRequired) || Boolean(token.value),
)

const lokiWarning = computed(() => {
  if (!meta.value || meta.value.lokiAvailable) return ''
  return '日志服务当前不可用，检索可能失败。请检查 Loki/Alloy 是否已启动。'
})

const summary = computed(() => {
  const current = result.value
  if (!current) return ''
  const parts = [
    `命中 ${current.matched} 条`,
    `${current.streams} 个日志来源`,
    `共 ${current.entries.length} 行`,
  ]
  if (elapsedMs.value > 0) parts.push(`${elapsedMs.value} ms`)
  return parts.join(' · ')
})

function formatTime(raw: string): string {
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  const pad = (value: number, width = 2) => String(value).padStart(width, '0')
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}.` +
    pad(date.getMilliseconds(), 3)
  )
}

function levelClass(level: string): string {
  switch (level.toLowerCase()) {
    case 'error':
    case 'fatal':
      return 'level--err'
    case 'warn':
    case 'warning':
      return 'level--warn'
    case 'debug':
    case 'trace':
      return 'level--dim'
    default:
      return 'level--info'
  }
}

function envLabel(value: string): string {
  if (value === 'production') return '生产'
  if (value === 'test') return '测试'
  if (value === 'all') return '全部'
  return value
}

function entryKey(entry: LogEntry): string {
  return `${entry.timestampNs}:${entry.line}`
}

function toggleEntry(key: string) {
  const next = new Set(expanded.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expanded.value = next
}

async function copyLine(entry: LogEntry) {
  const key = entryKey(entry)
  try {
    await navigator.clipboard.writeText(entry.line)
    copiedKey.value = key
    window.setTimeout(() => {
      if (copiedKey.value === key) copiedKey.value = ''
    }, 1500)
  } catch {
    error.value = '复制失败，请展开“原始”后手动选择文本'
  }
}

async function loadMeta() {
  metaError.value = ''
  try {
    meta.value = await fetchLogMeta()
  } catch (value) {
    metaError.value =
      value instanceof LogApiError
        ? `日志检索信息加载失败：${value.message}`
        : '日志检索信息加载失败，仍可直接尝试查询'
  }
}

async function submit() {
  if (loading.value) return
  error.value = ''

  const id = normalizeLogId(idInput.value)
  if (!id) {
    result.value = null
    error.value = '请输入 8-64 位十六进制链路编号，也可以直接粘贴带“编号”的错误提示'
    return
  }
  idInput.value = id

  if (meta.value?.authRequired && !token.value.trim()) {
    tokenNeeded.value = true
    error.value = '请输入日志检索口令'
    return
  }

  loading.value = true
  const started = performance.now()
  try {
    const response = await searchLogs(
      { id, env: env.value, lines: lines.value, range: range.value },
      token.value.trim(),
    )
    result.value = response
    elapsedMs.value = Math.round(performance.now() - started)
    expanded.value = new Set()
    tokenNeeded.value = false
    if (token.value.trim()) setLogToken(token.value.trim())
    await router.replace({
      query: {
        id,
        env: env.value,
        lines: String(lines.value),
        range: range.value,
      },
    })
  } catch (value) {
    result.value = null
    if (value instanceof LogApiError) {
      if (value.status === 401) {
        tokenNeeded.value = true
        clearLogToken()
        error.value = '日志检索口令无效，请重新输入'
      } else {
        error.value = value.message
      }
    } else {
      error.value = '日志检索失败，请稍后重试'
    }
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await loadMeta()
  if (!route.query.id) return
  if (meta.value?.authRequired && !token.value.trim()) {
    tokenNeeded.value = true
    return
  }
  await submit()
})
</script>

<template>
  <main class="logs">
    <section class="logs__intro">
      <div>
        <div class="logs__eyebrow">OBSERVABILITY / TRACE LOOKUP</div>
        <h1>日志检索</h1>
        <p>输入错误提示中的编号，查看同一次请求在网关与各服务中的完整日志。</p>
      </div>
      <div class="logs__hints">
        <span>编号来自响应头 X-Trace-ID</span>
        <span>日志保留 {{ meta?.retentionDays ?? 14 }} 天</span>
      </div>
    </section>

    <div v-if="metaError" class="banner banner--warn">{{ metaError }}</div>
    <div v-if="lokiWarning" class="banner banner--warn">{{ lokiWarning }}</div>

    <form class="logs__form" @submit.prevent="submit">
      <label class="field field--id">
        <span>链路编号 / 请求 ID</span>
        <input
          v-model="idInput"
          class="logs__input"
          placeholder="例如 4bf92f3577b34da6a3ce929d0e0e4736"
          autocomplete="off"
          spellcheck="false"
        />
      </label>
      <label class="field">
        <span>环境</span>
        <select v-model="env">
          <option v-for="option in ENV_OPTIONS" :key="option.value" :value="option.value">
            {{ option.label }}
          </option>
        </select>
      </label>
      <label class="field">
        <span>上下文</span>
        <select v-model="lines">
          <option v-for="option in LINES_OPTIONS" :key="option.value" :value="option.value">
            {{ option.label }}
          </option>
        </select>
      </label>
      <label class="field">
        <span>时间范围</span>
        <select v-model="range">
          <option v-for="option in RANGE_OPTIONS" :key="option.value" :value="option.value">
            {{ option.label }}
          </option>
        </select>
      </label>
      <button class="logs__submit" type="submit" :disabled="loading">
        {{ loading ? '检索中…' : '检索日志' }}
      </button>
    </form>

    <label v-if="showToken" class="field logs__token">
      <span>检索口令</span>
      <input
        v-model="token"
        type="password"
        placeholder="输入日志检索口令"
        autocomplete="off"
        @keydown.enter.prevent="submit"
      />
      <small v-if="meta?.authRequired">口令只保存在当前标签页，关闭浏览器后自动清除。</small>
    </label>

    <p v-if="error" class="logs__error">{{ error }}</p>

    <section v-if="result" class="logs__result">
      <header class="logs__summary">
        <div class="logs__summary-main">
          <span class="logs__summary-id">{{ result.id }}</span>
          <span>{{ summary }}</span>
        </div>
        <span class="logs__range">{{ envLabel(result.env) }} · {{ result.range }}</span>
      </header>

      <div v-if="result.warnings?.length" class="logs__warn-line">
        {{ result.warnings.join('；') }}
      </div>
      <div v-if="result.truncated" class="logs__warn-line">
        结果达到上限，已截断。可缩小时间范围或环境后重试。
      </div>

      <div v-if="!result.entries.length" class="logs__empty">
        <p>没有找到该编号的日志。</p>
        <p>
          请确认编号、环境与时间范围；日志保留 {{ meta?.retentionDays ?? 14 }} 天，编号需为完整的
          trace ID。
        </p>
      </div>

      <div v-else class="logs__entries">
        <article
          v-for="entry in result.entries"
          :key="entryKey(entry)"
          class="log-row"
          :class="{ 'log-row--matched': entry.matched }"
        >
          <div class="log-row__head">
            <span class="log-row__time">{{ formatTime(entry.timestamp) }}</span>
            <span class="log-row__level" :class="levelClass(entry.level)">
              {{ entry.level.toUpperCase() }}
            </span>
            <span class="log-row__service">{{ entry.service || 'unknown' }}</span>
            <span v-if="entry.env" class="log-row__env">{{ entry.env }}</span>
            <span v-if="entry.matched" class="log-row__match">命中</span>
            <span class="log-row__spacer" />
            <button type="button" class="log-row__action" @click="copyLine(entry)">
              {{ copiedKey === entryKey(entry) ? '已复制' : '复制' }}
            </button>
            <button type="button" class="log-row__action" @click="toggleEntry(entryKey(entry))">
              {{ expanded.has(entryKey(entry)) ? '收起' : '原始' }}
            </button>
          </div>
          <pre class="log-row__message">{{ entry.message }}</pre>
          <pre v-if="expanded.has(entryKey(entry))" class="log-row__raw">{{ entry.line }}</pre>
        </article>
      </div>
    </section>
  </main>
</template>

<style scoped>
.logs {
  display: flex;
  flex-direction: column;
  gap: 14px;
  width: 100%;
  max-width: 1280px;
  margin: 0 auto;
  padding: 18px 20px 80px;
}

.logs__intro {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px;
}

.logs__eyebrow {
  color: var(--text-dim);
  font-family: var(--mono);
  font-size: 11px;
  letter-spacing: 0.08em;
}

.logs__intro h1 {
  margin: 6px 0 4px;
  font-size: 22px;
  font-weight: 600;
}

.logs__intro p {
  margin: 0;
  max-width: 720px;
  color: var(--text-dim);
  font-size: 13px;
  line-height: 1.6;
}

.logs__hints {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  color: var(--text-dim);
  font-size: 11px;
  font-family: var(--mono);
  white-space: nowrap;
}

.logs__form {
  display: grid;
  grid-template-columns: minmax(300px, 1fr) 120px 125px 140px 112px;
  gap: 10px;
  align-items: end;
  padding: 14px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--panel);
}

.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
  color: var(--text-dim);
  font-size: 12px;
}

.logs__input,
.field select,
.logs__token input {
  width: 100%;
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
  outline: none;
  background: var(--panel-2);
  color: var(--text);
  font-family: var(--mono);
  font-size: 12px;
}

.logs__input:focus,
.field select:focus,
.logs__token input:focus {
  border-color: var(--progress);
}

.logs__submit {
  height: 34px;
  border: 1px solid var(--progress);
  border-radius: 6px;
  background: var(--progress);
  color: #06121f;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.logs__submit:hover:not(:disabled) {
  filter: brightness(1.08);
}

.logs__submit:disabled {
  opacity: 0.6;
  cursor: wait;
}

.logs__token {
  max-width: 460px;
}

.logs__token small {
  color: var(--idle);
  font-size: 11px;
  line-height: 1.5;
}

.logs__error {
  margin: 0;
  padding: 9px 12px;
  border: 1px solid rgba(248, 81, 73, 0.4);
  border-radius: 6px;
  background: rgba(248, 81, 73, 0.1);
  color: var(--err);
  font-size: 12.5px;
}

.logs__result {
  border: 1px solid var(--border);
  border-radius: 8px;
  overflow: hidden;
  background: var(--panel);
}

.logs__summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--border);
  background: var(--panel-2);
  font-size: 12px;
  color: var(--text-dim);
}

.logs__summary-main {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.logs__summary-id {
  overflow: hidden;
  color: var(--progress);
  font-family: var(--mono);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.logs__range {
  flex-shrink: 0;
  font-family: var(--mono);
  font-size: 11px;
}

.logs__warn-line {
  padding: 7px 14px;
  border-bottom: 1px solid var(--border);
  background: rgba(210, 153, 34, 0.1);
  color: var(--warn);
  font-size: 12px;
}

.logs__empty {
  padding: 36px 20px;
  color: var(--text-dim);
  text-align: center;
  font-size: 13px;
  line-height: 1.7;
}

.logs__empty p {
  margin: 0;
}

.log-row {
  padding: 8px 14px;
  border-bottom: 1px solid var(--border);
}

.log-row:last-child {
  border-bottom: none;
}

.log-row:hover {
  background: var(--panel-2);
}

.log-row--matched {
  background: rgba(88, 166, 255, 0.07);
  box-shadow: inset 3px 0 var(--progress);
}

.log-row--matched:hover {
  background: rgba(88, 166, 255, 0.11);
}

.log-row__head {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 18px;
  font-size: 11px;
  color: var(--text-dim);
}

.log-row__time {
  flex-shrink: 0;
  font-family: var(--mono);
  color: var(--idle);
}

.log-row__level {
  flex-shrink: 0;
  font-family: var(--mono);
  font-weight: 600;
}

.level--info {
  color: var(--ok);
}

.level--warn {
  color: var(--warn);
}

.level--err {
  color: var(--err);
}

.level--dim {
  color: var(--idle);
}

.log-row__service {
  flex-shrink: 0;
  font-family: var(--mono);
  color: var(--text);
}

.log-row__env {
  flex-shrink: 0;
  padding: 0 6px;
  border: 1px solid var(--border);
  border-radius: 999px;
  font-size: 10px;
}

.log-row__match {
  flex-shrink: 0;
  color: var(--progress);
  font-weight: 600;
}

.log-row__spacer {
  flex: 1;
}

.log-row__action {
  padding: 1px 6px;
  border: 1px solid transparent;
  border-radius: 4px;
  background: transparent;
  color: var(--text-dim);
  font-size: 11px;
  cursor: pointer;
}

.log-row__action:hover {
  border-color: var(--border);
  color: var(--text);
}

.log-row__message,
.log-row__raw {
  margin: 5px 0 0;
  font-family: var(--mono);
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}

.log-row__message {
  color: var(--text);
}

.log-row__raw {
  max-height: 320px;
  margin-top: 7px;
  padding-top: 7px;
  overflow: auto;
  border-top: 1px dashed var(--border);
  color: var(--text-dim);
}

@media (max-width: 980px) {
  .logs__form {
    grid-template-columns: minmax(240px, 1fr) 120px 120px;
  }

  .logs__form .field--id {
    grid-column: span 3;
  }

  .logs__submit {
    grid-column: span 3;
  }

  .logs__intro {
    flex-direction: column;
    align-items: flex-start;
  }

  .logs__hints {
    align-items: flex-start;
  }
}

@media (max-width: 620px) {
  .logs {
    padding: 14px 12px 72px;
  }

  .logs__form {
    grid-template-columns: 1fr;
  }

  .logs__form .field--id,
  .logs__submit {
    grid-column: auto;
  }

  .logs__summary {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
  }

  .log-row__head {
    flex-wrap: wrap;
  }
}
</style>
