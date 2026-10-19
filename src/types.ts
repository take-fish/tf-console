// 与后端 model.go 的快照 JSON 一一对应（snake_case）
export interface Snapshot {
  generated_at: string
  poll_interval_seconds: number
  token_configured: boolean
  degraded: boolean
  degraded_reason: string
  rate_limit: RateLimitView
  repos: RepoStatus[]
  events: EventView[]
}

export interface RateLimitView {
  limit: number
  remaining: number
  reset_at: string
}

export interface RepoStatus {
  name: string
  html_url: string
  error: string
  stale: boolean
  last_success_at?: string
  latest_commit: CommitView | null
  test_branch: TestBranchView | null
  pull_requests: PRSection
  build: BuildView | null
  release: ReleaseView | null
}

export interface CommitView {
  sha: string
  short_sha: string
  message: string
  author: string
  author_avatar: string
  committed_at: string
  html_url: string
}

export interface BranchView {
  name: string
  short_sha: string
  protected: boolean
}

export interface TestBranchView {
  exists: boolean
  latest_commit: CommitView | null
  ahead: number
  behind: number
  status: string
  ci: CIView | null
}

export interface CIView {
  status: string
  conclusion: string
  html_url: string
  updated_at: string
}

export interface PRSection {
  open_count: number
  items: PRView[]
}

export interface PRView {
  number: number
  title: string
  author: string
  head: string
  base: string
  draft: boolean
  created_at: string
  updated_at: string
  html_url: string
  review_state: string
}

export interface BuildView {
  run_id: number
  status: string
  conclusion: string
  head_sha: string
  html_url: string
  created_at: string
  updated_at: string
  progress: ProgressView | null
  jobs: JobView[]
}

export interface ProgressView {
  completed_steps: number
  total_steps: number
  percent: number
  current_job: string
  current_step: string
}

export interface JobView {
  name: string
  status: string
  conclusion: string
  html_url: string
  steps: StepView[]
}

export interface StepView {
  number: number
  name: string
  status: string
  conclusion: string
}

export interface ReleaseView {
  tag: string
  name: string
  published_at: string
  html_url: string
  prerelease: boolean
}

export interface EventView {
  id: string
  type: string
  actor: string
  actor_avatar: string
  repo: string
  summary: string
  created_at: string
}

export type Tone = 'ok' | 'warn' | 'err' | 'progress' | 'idle'
