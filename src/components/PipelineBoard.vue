<script setup lang="ts">
import type { RepoStatus } from '@/types'
import RepoLane from '@/components/RepoLane.vue'

defineProps<{
  repos: RepoStatus[]
  loading: boolean
}>()
</script>

<template>
  <section class="board">
    <div class="board-head">
      <span>仓库</span>
      <span>最新提交</span>
      <span>test 联调</span>
      <span>开放 PR / 审核</span>
      <span>master 构建</span>
      <span>最新版本</span>
    </div>

    <template v-if="loading && repos.length === 0">
      <div v-for="i in 5" :key="i" class="lane lane--skeleton"><span class="skeleton" /></div>
    </template>

    <RepoLane v-for="repo in repos" :key="repo.name" :repo="repo" />

    <div v-if="!loading && repos.length === 0" class="empty">暂无数据（首轮抓取中）</div>
  </section>
</template>
