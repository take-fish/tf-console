<script setup lang="ts">
import { computed, ref } from 'vue'
import type { BuildView, Tone } from '@/types'
import { clampPercent } from '@/utils/format'

const props = defineProps<{ build: BuildView | null }>()

const expanded = ref(false)

const tone = computed<Tone>(() => {
  const b = props.build
  if (!b) return 'idle'
  if (b.status !== 'completed') return 'progress'
  return b.conclusion === 'success' ? 'ok' : 'err'
})

const label = computed(() => {
  const b = props.build
  if (!b) return '—'
  if (b.status === 'queued') return '排队中'
  if (b.status === 'in_progress') return `构建中 ${clampPercent(b.progress?.percent ?? 0)}%`
  if (b.conclusion === 'success') return '成功'
  return b.conclusion || '完成'
})

const running = computed(() => {
  const b = props.build
  return !!b && b.status !== 'completed'
})
</script>

<template>
  <div v-if="!build" class="chip chip--idle">—</div>
  <div v-else class="build">
    <a
      class="chip"
      :class="`chip--${tone}`"
      :href="build.html_url"
      target="_blank"
      rel="noreferrer"
      :title="`${build.head_sha ? build.head_sha.slice(0, 7) : ''} · ${build.status}${build.conclusion ? ' / ' + build.conclusion : ''}`"
    >
      <span v-if="running" class="spinner" />
      {{ label }}
    </a>
    <button v-if="build.jobs.length > 0" class="expand" @click="expanded = !expanded">
      {{ expanded ? '收起' : '详情' }}
    </button>

    <template v-if="running && build.progress">
      <div class="bar">
        <div class="bar-fill" :style="{ width: clampPercent(build.progress.percent) + '%' }" />
      </div>
      <div class="step-text">
        {{ build.progress.current_step }}（{{ build.progress.completed_steps }}/{{
          build.progress.total_steps
        }}）
      </div>
    </template>

    <div v-if="expanded" class="jobs">
      <div v-for="j in build.jobs" :key="j.name" class="job">
        <span class="job-name">{{ j.name }}</span>
        <span
          v-for="s in j.steps"
          :key="s.number"
          class="step"
          :class="`step--${running ? (s.status === 'completed' ? (s.conclusion === 'success' ? 'ok' : 'err') : s.status === 'in_progress' ? 'run' : 'idle') : s.conclusion === 'success' ? 'ok' : 'err'}`"
          :title="`${s.name}${s.conclusion ? ' · ' + s.conclusion : ''}`"
        />
      </div>
    </div>
  </div>
</template>
