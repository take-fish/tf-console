<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { usePipelineStore } from '@/stores/pipeline'

const store = usePipelineStore()

const pollSeconds = computed(() => store.snapshot?.poll_interval_seconds ?? 30)
const updatedText = computed(() =>
  store.lastUpdated ? store.lastUpdated.toLocaleTimeString('zh-CN') : '-',
)
</script>

<template>
  <header class="topbar">
    <RouterLink to="/" class="brand">
      FishMatrix <span>流水线中台</span>
    </RouterLink>
    <div class="meta">
      <RouterLink to="/logs" class="nav-link" title="按链路编号检索日志">日志检索</RouterLink>
      <span v-if="store.rateText">{{ store.rateText }}</span>
      <span>轮询 {{ pollSeconds }}s</span>
      <span>更新于 {{ updatedText }}</span>
      <button class="refresh" @click="store.refresh">刷新</button>
    </div>
  </header>
</template>
