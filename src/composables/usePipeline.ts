import { onMounted, onUnmounted, ref } from 'vue'
import type { Snapshot } from '@/types'

// 轮询 /api/v1/pipeline：出错时保留上一次数据，页面隐藏时暂停
export function usePipeline(url = '/api/v1/pipeline', intervalMs = 20000) {
  const snapshot = ref<Snapshot | null>(null)
  const loading = ref(true)
  const error = ref<string | null>(null)
  const lastUpdated = ref<Date | null>(null)

  let timer: number | undefined
  let stopped = false

  async function refresh() {
    try {
      const resp = await fetch(url)
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
    }, intervalMs)
  }

  function onVisibility() {
    // 切回页面时立即刷新一次，避免看到过期数据
    if (!document.hidden) void refresh()
  }

  onMounted(() => {
    void refresh().then(schedule)
    document.addEventListener('visibilitychange', onVisibility)
  })

  onUnmounted(() => {
    stopped = true
    if (timer !== undefined) window.clearTimeout(timer)
    document.removeEventListener('visibilitychange', onVisibility)
  })

  return { snapshot, loading, error, lastUpdated, refresh }
}
