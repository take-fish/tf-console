<script setup lang="ts">
import type { EventView } from '@/types'
import { relativeTime } from '@/utils/format'

defineProps<{ events: EventView[] }>()

function iconOf(type: string): string {
  if (type === 'PushEvent') return '⬆'
  if (type === 'PullRequestEvent' || type === 'PullRequestReviewEvent') return '⇄'
  if (type === 'ReleaseEvent') return '★'
  if (type === 'CreateEvent') return '＋'
  if (type === 'DeleteEvent') return '－'
  if (type === 'PublicEvent') return '◎'
  return '·'
}
</script>

<template>
  <footer class="ticker">
    <span class="ticker-label">动态</span>
    <span class="ticker-note" title="GitHub 事件流官方存在 30 秒 ~ 数小时延迟">事件有延迟</span>
    <div class="ticker-scroll">
      <span v-for="e in events" :key="e.id" class="ticker-item">
        <span class="t-time">{{ relativeTime(e.created_at) }}</span>
        <span>{{ iconOf(e.type) }}</span>
        <span class="t-actor">{{ e.actor }}</span>
        <span class="t-repo">@{{ e.repo }}</span>
        <span class="t-summary">{{ e.summary }}</span>
      </span>
      <span v-if="events.length === 0" class="ticker-item">暂无事件</span>
    </div>
  </footer>
</template>
