<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { RouterView } from 'vue-router'

import AppHeader from '@/components/AppHeader.vue'
import EventTicker from '@/components/EventTicker.vue'
import { usePipelineStore } from '@/stores/pipeline'

const store = usePipelineStore()
const events = computed(() => store.events)

onMounted(() => store.start())
onUnmounted(() => store.stop())
</script>

<template>
  <div class="app">
    <AppHeader />

    <div v-if="!store.tokenConfigured" class="banner banner--warn">
      未配置 GITHUB_TOKEN：{{ store.degradedReason || '轮询已降频，数据可能滞后' }}
    </div>
    <div v-if="store.error" class="banner banner--err">
      拉取失败：{{ store.error }}（显示上一次数据）
    </div>

    <RouterView />

    <EventTicker :events="events" />
  </div>
</template>
