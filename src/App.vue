<script setup lang="ts">
import { computed } from 'vue'
import PipelineBoard from '@/components/PipelineBoard.vue'
import EventTicker from '@/components/EventTicker.vue'
import { usePipeline } from '@/composables/usePipeline'

const { snapshot, loading, error, lastUpdated, refresh } = usePipeline()

const repos = computed(() => snapshot.value?.repos ?? [])
const events = computed(() => snapshot.value?.events ?? [])
const tokenConfigured = computed(() => snapshot.value?.token_configured ?? true)
const degradedReason = computed(() => snapshot.value?.degraded_reason ?? '')
const rateText = computed(() => {
  const r = snapshot.value?.rate_limit
  return r && r.limit > 0 ? `配额 ${r.remaining}/${r.limit}` : ''
})
</script>

<template>
  <div class="app">
    <header class="topbar">
      <div class="brand">FishMatrix <span>流水线控制台</span></div>
      <div class="meta">
        <span v-if="rateText">{{ rateText }}</span>
        <span>轮询 {{ snapshot?.poll_interval_seconds ?? 30 }}s</span>
        <span>更新于 {{ lastUpdated ? lastUpdated.toLocaleTimeString('zh-CN') : '-' }}</span>
        <button class="refresh" @click="refresh">刷新</button>
      </div>
    </header>

    <div v-if="!tokenConfigured" class="banner banner--warn">
      未配置 GITHUB_TOKEN：{{ degradedReason || '轮询已降频，数据可能滞后' }}
    </div>
    <div v-if="error" class="banner banner--err">拉取失败：{{ error }}（显示上一次数据）</div>

    <main>
      <PipelineBoard :repos="repos" :loading="loading" />
    </main>

    <EventTicker :events="events" />
  </div>
</template>
