<script setup lang="ts">
import { computed } from 'vue'
import type { RepoStatus, Tone } from '@/types'
import StageChip from '@/components/StageChip.vue'
import BuildProgress from '@/components/BuildProgress.vue'
import { relativeTime } from '@/utils/format'

const props = defineProps<{ repo: RepoStatus }>()

interface ChipSpec {
  value: string
  tone: Tone
  href?: string
  hint?: string
}

const commitChip = computed<ChipSpec>(() => {
  const c = props.repo.latest_commit
  if (!c) return { value: '—', tone: 'idle' }
  return {
    value: `${c.short_sha} ${c.author || ''}`.trim(),
    tone: 'idle',
    href: c.html_url,
    hint: `${c.message}\n${relativeTime(c.committed_at)}`,
  }
})

const testChip = computed<ChipSpec>(() => {
  const t = props.repo.test_branch
  if (!t || !t.exists) return { value: '缺失', tone: 'idle', hint: 'test 分支不存在' }
  // 进行中的 CI 优先展示
  if (t.ci && (t.ci.status === 'in_progress' || t.ci.status === 'queued')) {
    return { value: 'CI 构建中…', tone: 'progress', href: t.ci.html_url, hint: 'test 分支 CI 运行中' }
  }
  if (t.ci && t.ci.conclusion === 'failure') {
    return { value: 'CI 失败', tone: 'err', href: t.ci.html_url, hint: 'test 分支 CI 失败' }
  }
  const hint = `test 相对 master：领先 ${t.ahead} / 落后 ${t.behind}`
  switch (t.status) {
    case 'synced':
      return { value: '同步', tone: 'ok', hint }
    case 'ahead':
      return { value: `领先 ${t.ahead}`, tone: 'progress', hint }
    case 'behind':
      return { value: `落后 ${t.behind}`, tone: 'warn', hint }
    case 'diverged':
      return { value: `分叉 +${t.ahead}/-${t.behind}`, tone: 'idle', hint }
    default:
      return { value: t.status, tone: 'idle', hint }
  }
})

const prChip = computed<ChipSpec>(() => {
  const sec = props.repo.pull_requests
  if (!sec || sec.open_count === 0) return { value: '无待审', tone: 'idle' }
  const first = sec.items[0]
  if (!first) return { value: `待审 ${sec.open_count} 个`, tone: 'warn' }
  const more = sec.open_count > 1 ? ` +${sec.open_count - 1}` : ''
  const hint = `${first.head} → ${first.base}\n${first.title}`
  switch (first.review_state) {
    case 'approved':
      return { value: `已通过 #${first.number}${more}`, tone: 'ok', href: first.html_url, hint }
    case 'changes_requested':
      return { value: `要改 #${first.number}${more}`, tone: 'err', href: first.html_url, hint }
    default:
      return { value: `待审核 #${first.number}${more}`, tone: 'warn', href: first.html_url, hint }
  }
})

const releaseChip = computed<ChipSpec>(() => {
  const r = props.repo.release
  if (!r) return { value: '—', tone: 'idle' }
  return { value: r.tag, tone: 'ok', href: r.html_url, hint: `发布于 ${relativeTime(r.published_at)}` }
})
</script>

<template>
  <div class="lane" :class="{ 'lane--stale': repo.stale }">
    <div class="col col--repo">
      <a class="repo-name" :href="repo.html_url" target="_blank" rel="noreferrer">{{ repo.name }}</a>
      <span v-if="repo.error" class="repo-err" :title="repo.error">!</span>
    </div>
    <div class="col col--commit">
      <StageChip v-bind="commitChip" />
      <div class="commit-msg" :title="repo.latest_commit?.message">
        {{ repo.latest_commit?.message ?? '' }}
      </div>
    </div>
    <div class="col col--test"><StageChip v-bind="testChip" /></div>
    <div class="col col--pr"><StageChip v-bind="prChip" /></div>
    <div class="col col--build"><BuildProgress :build="repo.build" /></div>
    <div class="col col--release"><StageChip v-bind="releaseChip" /></div>
  </div>
</template>
