import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { RepoStatus, Snapshot } from '@/types'

const DEFAULT_INTERVAL_MS = 20_000

export const usePipelineStore = defineStore('pipeline', () => {
  const snapshot = ref<Snapshot | null>(null)
  const loading = ref(true)
  const error = ref<string | null>(null)
  const lastUpdated = ref<Date | null>(null)

  let timer: number | undefined
  let stopped = true
  let visibilityBound = false

  async function refresh() {
    try {
      const resp = await fetch('/api/v1/pipeline')
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
      snapshot.value = (await resp.json()) as Snapshot
      error.value = null
      lastUpdated.value = new Date()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  function schedule() {
    if (stopped) return
    timer = window.setTimeout(async () => {
      if (!document.hidden) await refresh()
      schedule()
    }, DEFAULT_INTERVAL_MS)
  }

  function onVisibility() {
    if (!document.hidden) void refresh()
  }

  function start() {
    if (!stopped) return
    stopped = false
    if (!visibilityBound) {
      document.addEventListener('visibilitychange', onVisibility)
      visibilityBound = true
    }
    void refresh().then(schedule)
  }

  function stop() {
    stopped = true
    if (timer !== undefined) {
      window.clearTimeout(timer)
      timer = undefined
    }
    if (visibilityBound) {
      document.removeEventListener('visibilitychange', onVisibility)
      visibilityBound = false
    }
  }

  const repos = computed(() => snapshot.value?.repos ?? [])
  const events = computed(() => snapshot.value?.events ?? [])
  const tokenConfigured = computed(() => snapshot.value?.token_configured ?? true)
  const degradedReason = computed(() => snapshot.value?.degraded_reason ?? '')
  const rateText = computed(() => {
    const rate = snapshot.value?.rate_limit
    return rate && rate.limit > 0 ? `配额 ${rate.remaining}/${rate.limit}` : ''
  })

  function repoByName(name: string): RepoStatus | null {
    return repos.value.find((repo) => repo.name === name) ?? null
  }

  return {
    snapshot,
    loading,
    error,
    lastUpdated,
    repos,
    events,
    tokenConfigured,
    degradedReason,
    rateText,
    repoByName,
    refresh,
    start,
    stop,
  }
})
