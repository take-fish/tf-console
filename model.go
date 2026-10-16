package main

import (
	"encoding/json"
	"strings"
	"time"
)

// ─── 对外快照 JSON（字段与前端 src/types.ts 对齐，snake_case）───

// Snapshot GET /api/v1/pipeline 返回的完整快照
type Snapshot struct {
	GeneratedAt     time.Time     `json:"generated_at"`
	PollIntervalSec int           `json:"poll_interval_seconds"`
	TokenConfigured bool          `json:"token_configured"`
	Degraded        bool          `json:"degraded"`
	DegradedReason  string        `json:"degraded_reason"`
	RateLimit       RateLimitView `json:"rate_limit"`
	Repos           []RepoStatus  `json:"repos"`
	Events          []EventView   `json:"events"`
}

// RateLimitView 配额视图
type RateLimitView struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"reset_at"`
}

// RepoStatus 单仓五阶段状态
type RepoStatus struct {
	Name          string          `json:"name"`
	HTMLURL       string          `json:"html_url"`
	Error         string          `json:"error"` // 非空表示本轮该仓部分数据获取失败
	Stale         bool            `json:"stale"` // true = 沿用上一轮数据
	LastSuccessAt *time.Time      `json:"last_success_at,omitempty"`
	LatestCommit  *CommitView     `json:"latest_commit"`
	TestBranch    *TestBranchView `json:"test_branch"`
	PullRequests  PRSection       `json:"pull_requests"`
	Build         *BuildView      `json:"build"`
	Release       *ReleaseView    `json:"release"`
}

// CommitView master 最新提交（阶段 1）
type CommitView struct {
	SHA          string    `json:"sha"`
	ShortSHA     string    `json:"short_sha"`
	Message      string    `json:"message"`
	Author       string    `json:"author"`
	AuthorAvatar string    `json:"author_avatar"`
	CommittedAt  time.Time `json:"committed_at"`
	HTMLURL      string    `json:"html_url"`
}

// TestBranchView test 联调分支状态（阶段 2）
type TestBranchView struct {
	Exists       bool        `json:"exists"`
	LatestCommit *CommitView `json:"latest_commit"`
	Ahead        int         `json:"ahead"`
	Behind       int         `json:"behind"`
	Status       string      `json:"status"` // synced|ahead|behind|diverged|missing
	CI           *CIView     `json:"ci"`
}

// CIView test 分支最近一次 CI 状态
type CIView struct {
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	HTMLURL    string    `json:"html_url"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// PRSection 开放 PR 与审核状态（阶段 3）
type PRSection struct {
	OpenCount int      `json:"open_count"`
	Items     []PRView `json:"items"`
}

// PRView 单个开放 PR
type PRView struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	Head        string    `json:"head"`
	Base        string    `json:"base"`
	Draft       bool      `json:"draft"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	HTMLURL     string    `json:"html_url"`
	ReviewState string    `json:"review_state"` // pending|approved|changes_requested
}

// BuildView master 最近一次 CI 运行与进度（阶段 4）
type BuildView struct {
	RunID      int64         `json:"run_id"`
	Status     string        `json:"status"`     // queued|in_progress|completed
	Conclusion string        `json:"conclusion"` // success|failure|cancelled|... 未完成时为空
	HeadSHA    string        `json:"head_sha"`
	HTMLURL    string        `json:"html_url"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
	Progress   *ProgressView `json:"progress"` // 仅未完成时有值
	Jobs       []JobView     `json:"jobs"`     // 仅未完成时下钻拉取
}

// ProgressView step 级进度
type ProgressView struct {
	CompletedSteps int    `json:"completed_steps"`
	TotalSteps     int    `json:"total_steps"`
	Percent        int    `json:"percent"`
	CurrentJob     string `json:"current_job"`
	CurrentStep    string `json:"current_step"`
}

// JobView 单个 workflow job
type JobView struct {
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	Conclusion string     `json:"conclusion"`
	HTMLURL    string     `json:"html_url"`
	Steps      []StepView `json:"steps"`
}

// StepView 单个 step
type StepView struct {
	Number     int    `json:"number"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// ReleaseView 最新 Release（阶段 5）
type ReleaseView struct {
	Tag         string    `json:"tag"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Prerelease  bool      `json:"prerelease"`
}

// EventView org 事件流条目
type EventView struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Actor       string    `json:"actor"`
	ActorAvatar string    `json:"actor_avatar"`
	Repo        string    `json:"repo"`
	Summary     string    `json:"summary"`
	CreatedAt   time.Time `json:"created_at"`
}

// ─── GitHub 线上类型（只解需要的字段）───

type ghCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string    `json:"name"`
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	Author *struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"author"`
	HTMLURL string `json:"html_url"`
}

type ghCompare struct {
	Status  string     `json:"status"` // ahead|behind|diverged|identical
	AheadBy int        `json:"ahead_by"`
	Behind  int        `json:"behind_by"`
	Commits []ghCommit `json:"commits"`
}

type ghUser struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

type ghPull struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Draft     bool      `json:"draft"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	User      *ghUser   `json:"user"`
	Head      struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

type ghReview struct {
	User        *ghUser   `json:"user"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
}

type ghRuns struct {
	WorkflowRuns []ghRun `json:"workflow_runs"`
}

type ghRun struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Event      string    `json:"event"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	HeadSHA    string    `json:"head_sha"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ghJobs struct {
	Jobs []ghJob `json:"jobs"`
}

type ghStep struct {
	Number     int    `json:"number"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type ghJob struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	Conclusion string   `json:"conclusion"`
	HTMLURL    string   `json:"html_url"`
	Steps      []ghStep `json:"steps"`
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
}

type ghEvent struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Actor struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"actor"`
	Repo struct {
		Name string `json:"name"`
	} `json:"repo"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// ─── 纯函数（便于单测）───

// PRStatus 按 GitHub 语义归纳审核状态：取每个用户最新一条 review，
// 忽略 COMMENTED/PENDING/DISMISSED；有 CHANGES_REQUESTED 则 changes_requested，
// 否则有 APPROVED 则 approved，否则 pending。
func PRStatus(reviews []ghReview) string {
	latest := map[string]ghReview{}
	for _, r := range reviews {
		if r.User == nil || r.User.Login == "" {
			continue
		}
		if cur, ok := latest[r.User.Login]; !ok || r.SubmittedAt.After(cur.SubmittedAt) {
			latest[r.User.Login] = r
		}
	}
	approved, changes := 0, 0
	for _, r := range latest {
		switch strings.ToUpper(r.State) {
		case "APPROVED":
			approved++
		case "CHANGES_REQUESTED":
			changes++
		}
	}
	switch {
	case changes > 0:
		return "changes_requested"
	case approved > 0:
		return "approved"
	default:
		return "pending"
	}
}

// PickCIRun 从运行列表挑出 CI（.github/workflows/ci.yml）最近的一次；
// master 上同时存在 Release 工作流的运行，必须按 path 过滤。
func PickCIRun(runs []ghRun) *ghRun {
	var best *ghRun
	for i := range runs {
		r := &runs[i]
		if r.Path != ".github/workflows/ci.yml" {
			continue
		}
		if best == nil || r.CreatedAt.After(best.CreatedAt) {
			best = r
		}
	}
	return best
}

// ComputeProgress 计算 step 级进度；全部 job 都未开始时返回 nil
func ComputeProgress(jobs []ghJob) *ProgressView {
	total, done := 0, 0
	p := &ProgressView{}
	for _, j := range jobs {
		for _, s := range j.Steps {
			total++
			if s.Status == "completed" {
				done++
			} else if p.CurrentStep == "" {
				p.CurrentJob, p.CurrentStep = j.Name, s.Name
			}
		}
	}
	if total == 0 {
		return nil
	}
	p.CompletedSteps, p.TotalSteps = done, total
	p.Percent = done * 100 / total
	return p
}

// ShortSHA 截断 sha 为 7 位
func ShortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// toCommitView 转换并规整提交信息（首行 + 100 字符截断）
func toCommitView(c ghCommit) *CommitView {
	if c.SHA == "" {
		return nil
	}
	msg := strings.SplitN(c.Commit.Message, "\n", 2)[0]
	if runes := []rune(msg); len(runes) > 100 {
		msg = string(runes[:100]) + "…"
	}
	v := &CommitView{
		SHA:         c.SHA,
		ShortSHA:    ShortSHA(c.SHA),
		Message:     msg,
		Author:      c.Commit.Author.Name,
		CommittedAt: c.Commit.Author.Date,
		HTMLURL:     c.HTMLURL,
	}
	if c.Author != nil {
		v.Author = c.Author.Login
		v.AuthorAvatar = c.Author.AvatarURL
	}
	return v
}
