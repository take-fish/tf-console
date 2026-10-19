<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'

import BuildProgress from '@/components/BuildProgress.vue'
import { fetchBranches, fetchCommits } from '@/api/repo'
import { usePipelineStore } from '@/stores/pipeline'
import { relativeTime } from '@/utils/format'
import { groupBranches, pickDefaultBranch } from '@/utils/branches'
import type { BranchView, CIView, CommitView, PRView } from '@/types'

type TabId = 'branches' | 'pulls' | 'builds'

const props = defineProps<{ repo: string }>()
const store = usePipelineStore()

const status = computed(() => store.repoByName(props.repo))
const repoUrl = computed(
  () => status.value?.html_url ?? `https://github.com/take-fish/${props.repo}`,
)

const branches = ref<BranchView[]>([])
const selectedBranch = ref('master')
const commits = ref<CommitView[]>([])
const branchesLoading = ref(false)
const commitsLoading = ref(false)
const branchesError = ref('')
const commitsError = ref('')
const activeTab = ref<TabId>('branches')
const onlyPending = ref(false)

const branchGroups = computed(() => groupBranches(branches.value))
const primaryBranches = computed(() =>
  [branchGroups.value.master, branchGroups.value.test].filter(
    (branch): branch is BranchView => branch !== null,
  ),
)
const latestCommit = computed(() => status.value?.latest_commit ?? null)
const testBranch = computed(() => status.value?.test_branch ?? null)
const testCI = computed(() => testBranch.value?.ci ?? null)
const masterBuild = computed(() => status.value?.build ?? null)
const release = computed(() => status.value?.release ?? null)
const openPRCount = computed(() => status.value?.pull_requests?.open_count ?? 0)
const pullRequests = computed(() => status.value?.pull_requests?.items ?? [])
const visiblePulls = computed(() =>
  onlyPending.value
    ? pullRequests.value.filter((pr) => pr.review_state !== 'approved')
    : pullRequests.value,
)
const pendingPRCount = computed(
  () => pullRequests.value.filter((pr) => pr.review_state !== 'approved').length,
)
const tabs = computed<{ id: TabId; label: string }[]>(() => [
  { id: 'branches', label: '分支与提交' },
  { id: 'pulls', label: `合并申请${openPRCount.value ? ` (${openPRCount.value})` : ''}` },
  { id: 'builds', label: '构建与发版' },
])

function errorText(value: unknown): string {
  return value instanceof Error ? value.message : '请求失败'
}

async function loadCommits() {
  commitsLoading.value = true
  commitsError.value = ''
  try {
    commits.value = await fetchCommits(props.repo, selectedBranch.value)
  } catch (value) {
    commits.value = []
    commitsError.value = errorText(value)
  } finally {
    commitsLoading.value = false
  }
}

async function loadBranches() {
  branchesLoading.value = true
  branchesError.value = ''
  try {
    branches.value = await fetchBranches(props.repo)
    if (!branches.value.some((branch) => branch.name === selectedBranch.value)) {
      selectedBranch.value = pickDefaultBranch(groupBranches(branches.value))
    }
    await loadCommits()
  } catch (value) {
    branches.value = []
    commits.value = []
    branchesError.value = errorText(value)
  } finally {
    branchesLoading.value = false
  }
}

async function selectBranch(name: string) {
  if (name === selectedBranch.value) return
  selectedBranch.value = name
  await loadCommits()
}

async function refreshAll() {
  await Promise.all([loadBranches(), store.refresh()])
}

function branchUrl(name: string): string {
  const encoded = name
    .split('/')
    .map((part) => encodeURIComponent(part))
    .join('/')
  return `${repoUrl.value}/tree/${encoded}`
}

function reviewLabel(pr: PRView): string {
  if (pr.draft) return '草稿'
  switch (pr.review_state) {
    case 'approved':
      return '已通过'
    case 'changes_requested':
      return '需修改'
    default:
      return '待审核'
  }
}

function reviewTone(pr: PRView): string {
  if (pr.draft) return 'idle'
  switch (pr.review_state) {
    case 'approved':
      return 'ok'
    case 'changes_requested':
      return 'err'
    default:
      return 'warn'
  }
}

function ciLabel(ci: CIView | null): string {
  if (!ci) return '尚无运行'
  if (ci.status === 'completed') {
    if (ci.conclusion === 'success') return '成功'
    if (ci.conclusion === 'failure') return '失败'
    return ci.conclusion || '完成'
  }
  if (ci.status === 'in_progress') return '运行中'
  if (ci.status === 'queued') return '排队中'
  return ci.status
}

function ciTone(ci: CIView | null): string {
  if (!ci) return 'idle'
  if (ci.status !== 'completed') return 'progress'
  return ci.conclusion === 'success' ? 'ok' : 'err'
}

watch(
  () => props.repo,
  () => {
    selectedBranch.value = 'master'
    branches.value = []
    commits.value = []
    branchesError.value = ''
    commitsError.value = ''
    activeTab.value = 'branches'
    onlyPending.value = false
    void loadBranches()
  },
  { immediate: true },
)
</script>

<template>
  <main class="repo-page">
    <header class="repo-head">
      <div class="repo-crumbs">
        <RouterLink to="/">中台总览</RouterLink>
        <span>/</span>
        <span>{{ repo }}</span>
      </div>
      <div class="repo-title-row">
        <div class="repo-title">
          <h1>{{ repo }}</h1>
          <span v-if="status?.stale" class="repo-badge repo-badge--warn">数据滞后</span>
          <span v-if="status?.error" class="repo-badge repo-badge--err" :title="status.error">
            部分数据异常
          </span>
        </div>
        <div class="repo-actions">
          <button v-if="release" class="repo-action" @click="activeTab = 'builds'">
            发版进度 {{ release.tag }}
          </button>
          <a class="repo-action" :href="repoUrl" target="_blank" rel="noreferrer">GitHub ↗</a>
          <button class="repo-action" @click="refreshAll">刷新</button>
        </div>
      </div>
      <div class="repo-stats">
        <span>master <b>{{ latestCommit?.short_sha ?? '—' }}</b></span>
        <span>test <b>{{ testCI ? ciLabel(testCI) : testBranch?.status ?? '缺失' }}</b></span>
        <span>待审合并 <b>{{ pendingPRCount }}</b></span>
        <span>最新发版 <b>{{ release?.tag ?? '—' }}</b></span>
        <span v-if="latestCommit" class="muted">
          最近提交 {{ relativeTime(latestCommit.committed_at) }}
        </span>
      </div>
    </header>

    <div class="repo-tabs">
      <button
        v-for="tab in tabs"
        :key="tab.id"
        class="repo-tab"
        :class="{ 'repo-tab--active': activeTab === tab.id }"
        @click="activeTab = tab.id"
      >
        {{ tab.label }}
      </button>
    </div>

    <section v-if="activeTab === 'branches'" class="repo-panel">
      <div class="repo-panel__head">
        <div>
          <b>分支与提交</b>
          <span class="muted">只读 · 自建分支由平台外 Git 推送</span>
        </div>
        <button class="mini-btn" :disabled="branchesLoading" @click="loadBranches">刷新</button>
      </div>

      <div v-if="branchesError" class="repo-inline-error">{{ branchesError }}</div>

      <div class="branch-layout">
        <aside class="branch-list">
          <div v-if="branchesLoading" class="repo-empty">正在读取分支…</div>
          <template v-else>
            <div class="branch-group">
              <div class="branch-group__title">主干与联调</div>
              <button
                v-for="branch in primaryBranches"
                :key="branch.name"
                class="branch-item"
                :class="{ 'branch-item--active': selectedBranch === branch.name }"
                @click="selectBranch(branch.name)"
              >
                <span class="branch-name">{{ branch.name }}</span>
                <span class="branch-sha">{{ branch.short_sha }}</span>
                <span v-if="branch.protected" class="branch-lock">protected</span>
              </button>
            </div>
            <div class="branch-group">
              <div class="branch-group__title">
                自建分支 <em>{{ branchGroups.custom.length }}</em>
              </div>
              <button
                v-for="branch in branchGroups.custom"
                :key="branch.name"
                class="branch-item"
                :class="{ 'branch-item--active': selectedBranch === branch.name }"
                @click="selectBranch(branch.name)"
              >
                <span class="branch-name">{{ branch.name }}</span>
                <span class="branch-sha">{{ branch.short_sha }}</span>
              </button>
              <div v-if="branchGroups.custom.length === 0" class="repo-empty">暂无自建分支</div>
            </div>
          </template>
        </aside>

        <div class="commit-panel">
          <div class="commit-panel__head">
            <span><b>{{ selectedBranch }}</b> 最近提交</span>
            <span class="commit-panel__actions">
              <a class="mini-link" :href="branchUrl(selectedBranch)" target="_blank" rel="noreferrer">
                在 GitHub 查看
              </a>
              <button class="mini-btn" :disabled="commitsLoading" @click="loadCommits">刷新</button>
            </span>
          </div>

          <div v-if="commitsLoading" class="repo-empty">正在读取提交记录…</div>
          <div v-else-if="commitsError" class="repo-inline-error">{{ commitsError }}</div>
          <div v-else-if="commits.length === 0" class="repo-empty">该分支暂无提交</div>
          <ul v-else class="commit-list">
            <li v-for="commit in commits" :key="commit.sha" class="commit-item">
              <img
                v-if="commit.author_avatar"
                class="commit-avatar"
                :src="commit.author_avatar"
                :alt="commit.author"
              />
              <span v-else class="commit-avatar commit-avatar--fallback">
                {{ (commit.author || '?').slice(0, 1).toUpperCase() }}
              </span>
              <div class="commit-body">
                <a class="commit-msg" :href="commit.html_url" target="_blank" rel="noreferrer">
                  {{ commit.message }}
                </a>
                <div class="commit-meta">
                  <b>{{ commit.author || 'unknown' }}</b>
                  <span>{{ commit.short_sha }}</span>
                  <span>{{ relativeTime(commit.committed_at) }}</span>
                </div>
              </div>
            </li>
          </ul>
        </div>
      </div>
    </section>

    <section v-else-if="activeTab === 'pulls'" class="repo-panel">
      <div class="repo-panel__head">
        <div>
          <b>合并申请</b>
          <span class="muted">{{ openPRCount }} 个开放 PR</span>
        </div>
        <label class="repo-filter">
          <input v-model="onlyPending" type="checkbox" />
          仅看待审
        </label>
      </div>
      <p class="readonly-note">
        只读模式：登录与审批功能将在下一阶段开放；当前可前往 GitHub 完成审核与合并。
      </p>

      <div v-if="!status" class="repo-empty">正在同步流水线快照…</div>
      <div v-else-if="visiblePulls.length === 0" class="repo-empty">
        {{ pullRequests.length === 0 ? '暂无合并申请' : '当前没有待审申请' }}
      </div>
      <div v-else class="pr-list">
        <article v-for="pr in visiblePulls" :key="pr.number" class="pr-item">
          <div class="pr-main">
            <a class="pr-title" :href="pr.html_url" target="_blank" rel="noreferrer">
              #{{ pr.number }} {{ pr.title }}
            </a>
            <div class="pr-meta">
              {{ pr.author || 'unknown' }} · {{ pr.head }} → {{ pr.base }} · 更新于
              {{ relativeTime(pr.updated_at) }}
            </div>
          </div>
          <span class="repo-badge" :class="`repo-badge--${reviewTone(pr)}`">
            {{ reviewLabel(pr) }}
          </span>
        </article>
      </div>
    </section>

    <section v-else class="repo-panel">
      <div class="repo-panel__head">
        <div>
          <b>构建与发版</b>
          <span class="muted">数据来自 GitHub Actions</span>
        </div>
        <a class="mini-link" :href="`${repoUrl}/actions`" target="_blank" rel="noreferrer">
          打开 Actions
        </a>
      </div>
      <p class="readonly-note">
        本期展示 test CI 与 master Release 进度；集中机容器部署状态暂不包含。
      </p>

      <div class="build-grid">
        <article class="build-card">
          <div class="build-card__head">
            <b>test 联调 CI</b>
            <span class="repo-badge" :class="`repo-badge--${ciTone(testCI)}`">
              {{ ciLabel(testCI) }}
            </span>
          </div>
          <div v-if="testBranch?.exists" class="build-card__body">
            <p class="muted">
              相对 master：领先 {{ testBranch.ahead }} / 落后 {{ testBranch.behind }}
            </p>
            <p v-if="testCI" class="muted">更新于 {{ relativeTime(testCI.updated_at) }}</p>
            <a v-if="testCI" class="build-link" :href="testCI.html_url" target="_blank" rel="noreferrer">
              查看 CI 运行 →
            </a>
            <p v-else class="muted">test 分支尚无 CI 运行</p>
          </div>
          <p v-else class="muted">test 分支缺失</p>
        </article>

        <article class="build-card">
          <div class="build-card__head">
            <b>master 构建</b>
            <span v-if="masterBuild" class="muted">
              {{ masterBuild.status }}<template v-if="masterBuild.conclusion"> / {{ masterBuild.conclusion }}</template>
            </span>
          </div>
          <div class="build-card__body">
            <BuildProgress v-if="masterBuild" :build="masterBuild" />
            <p v-else class="muted">暂无 master 构建记录</p>
            <a
              v-if="masterBuild"
              class="build-link"
              :href="masterBuild.html_url"
              target="_blank"
              rel="noreferrer"
            >
              查看构建详情 →
            </a>
          </div>
        </article>

        <article class="build-card">
          <div class="build-card__head">
            <b>最新发布</b>
            <span v-if="release" class="repo-badge repo-badge--ok">{{ release.tag }}</span>
          </div>
          <div v-if="release" class="build-card__body">
            <p class="release-tag">{{ release.name || release.tag }}</p>
            <p class="muted">发布于 {{ relativeTime(release.published_at) }}</p>
            <a class="build-link" :href="release.html_url" target="_blank" rel="noreferrer">
              打开 Release →
            </a>
          </div>
          <p v-else class="muted">暂无 Release</p>
        </article>
      </div>
    </section>
  </main>
</template>
