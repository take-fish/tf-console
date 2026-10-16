package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// fetchRepo 拉取单仓全量状态；单个端点失败不阻断整仓，错误聚合到 Status.Error
func (p *Poller) fetchRepo(ctx context.Context, repo string) RepoStatus {
	st := RepoStatus{
		Name:    repo,
		HTMLURL: fmt.Sprintf("https://github.com/%s/%s", p.cfg.Owner, repo),
	}
	var errs []string
	collect := func(what string, err error) {
		if err != nil {
			errs = append(errs, what+": "+err.Error())
		}
	}

	var err error
	st.LatestCommit, err = p.fetchLatestCommit(ctx, repo, "master")
	collect("commit", err)

	st.TestBranch, err = p.fetchTestBranch(ctx, repo)
	collect("test", err)

	st.PullRequests, err = p.fetchPRs(ctx, repo)
	collect("pr", err)

	st.Build, err = p.fetchBuild(ctx, repo)
	collect("build", err)

	st.Release, err = p.fetchRelease(ctx, repo)
	collect("release", err)

	if len(errs) > 0 {
		st.Error = strings.Join(errs, "; ")
	}
	return st
}

// fetchLatestCommit 分支最新提交（空仓 404 视为无数据）。
// 注意：必须用列表端点 /commits?sha=<branch>（返回数组）；
// /commits/<ref> 是"获取单个 commit"端点（返回对象），不能用于此场景。
func (p *Poller) fetchLatestCommit(ctx context.Context, repo, branch string) (*CommitView, error) {
	var commits []ghCommit
	path := fmt.Sprintf("/repos/%s/%s/commits?sha=%s&per_page=1", p.cfg.Owner, repo, branch)
	if err := p.gh.getJSON(ctx, path, &commits); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if len(commits) == 0 {
		return nil, nil
	}
	return toCommitView(commits[0]), nil
}

// fetchTestBranch test 相对 master 的领先/落后与最近 CI
func (p *Poller) fetchTestBranch(ctx context.Context, repo string) (*TestBranchView, error) {
	var cmp ghCompare
	path := fmt.Sprintf("/repos/%s/%s/compare/master...test", p.cfg.Owner, repo)
	if err := p.gh.getJSON(ctx, path, &cmp); err != nil {
		if errors.Is(err, ErrNotFound) {
			return &TestBranchView{Exists: false, Status: "missing"}, nil
		}
		return nil, err
	}
	tv := &TestBranchView{
		Exists: true,
		Ahead:  cmp.AheadBy,
		Behind: cmp.Behind,
		Status: mapCompareStatus(cmp.Status),
	}
	// compare 的 commits 按时间升序，最后一条即 test 分支的 tip
	if n := len(cmp.Commits); n > 0 {
		tv.LatestCommit = toCommitView(cmp.Commits[n-1])
	}
	if p.cfg.TestCI && tv.Exists {
		ci, err := p.fetchTestCI(ctx, repo)
		if err != nil {
			return tv, err
		}
		tv.CI = ci
	}
	return tv, nil
}

func (p *Poller) fetchTestCI(ctx context.Context, repo string) (*CIView, error) {
	var runs ghRuns
	path := fmt.Sprintf("/repos/%s/%s/actions/runs?branch=test&per_page=5", p.cfg.Owner, repo)
	if err := p.gh.getJSON(ctx, path, &runs); err != nil {
		return nil, err
	}
	r := PickCIRun(runs.WorkflowRuns)
	if r == nil {
		return nil, nil
	}
	return &CIView{
		Status:     r.Status,
		Conclusion: r.Conclusion,
		HTMLURL:    r.HTMLURL,
		UpdatedAt:  r.UpdatedAt,
	}, nil
}

// fetchPRs 开放 PR 列表 + 每个 PR 的审核状态（最多下钻 5 个 PR 的 reviews）
func (p *Poller) fetchPRs(ctx context.Context, repo string) (PRSection, error) {
	var pulls []ghPull
	path := fmt.Sprintf("/repos/%s/%s/pulls?state=open&per_page=20", p.cfg.Owner, repo)
	if err := p.gh.getJSON(ctx, path, &pulls); err != nil {
		return PRSection{}, err
	}
	sec := PRSection{OpenCount: len(pulls), Items: []PRView{}}
	for i, pr := range pulls {
		rv := PRView{
			Number:      pr.Number,
			Title:       pr.Title,
			Head:        pr.Head.Ref,
			Base:        pr.Base.Ref,
			Draft:       pr.Draft,
			CreatedAt:   pr.CreatedAt,
			UpdatedAt:   pr.UpdatedAt,
			HTMLURL:     pr.HTMLURL,
			ReviewState: "pending",
		}
		if pr.User != nil {
			rv.Author = pr.User.Login
		}
		if i < 5 {
			var reviews []ghReview
			rpath := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews?per_page=100", p.cfg.Owner, repo, pr.Number)
			if err := p.gh.getJSON(ctx, rpath, &reviews); err == nil {
				rv.ReviewState = PRStatus(reviews)
			}
		}
		sec.Items = append(sec.Items, rv)
	}
	return sec, nil
}

// fetchBuild master 上最近一次 CI 运行；未完成时下钻 jobs 计算 step 级进度
func (p *Poller) fetchBuild(ctx context.Context, repo string) (*BuildView, error) {
	var runs ghRuns
	path := fmt.Sprintf("/repos/%s/%s/actions/runs?branch=master&per_page=10", p.cfg.Owner, repo)
	if err := p.gh.getJSON(ctx, path, &runs); err != nil {
		return nil, err
	}
	r := PickCIRun(runs.WorkflowRuns)
	if r == nil {
		return nil, nil
	}
	bv := &BuildView{
		RunID:      r.ID,
		Status:     r.Status,
		Conclusion: r.Conclusion,
		HeadSHA:    r.HeadSHA,
		HTMLURL:    r.HTMLURL,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
		Jobs:       []JobView{},
	}
	if r.Status != "completed" {
		var jobs ghJobs
		jpath := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs?per_page=20", p.cfg.Owner, repo, r.ID)
		if err := p.gh.getJSON(ctx, jpath, &jobs); err == nil {
			bv.Jobs = convertJobs(jobs.Jobs)
			bv.Progress = ComputeProgress(jobs.Jobs)
		}
	}
	return bv, nil
}

func (p *Poller) fetchRelease(ctx context.Context, repo string) (*ReleaseView, error) {
	var rel ghRelease
	path := fmt.Sprintf("/repos/%s/%s/releases/latest", p.cfg.Owner, repo)
	if err := p.gh.getJSON(ctx, path, &rel); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ReleaseView{
		Tag:         rel.TagName,
		Name:        rel.Name,
		PublishedAt: rel.PublishedAt,
		HTMLURL:     rel.HTMLURL,
		Prerelease:  rel.Prerelease,
	}, nil
}

func mapCompareStatus(s string) string {
	switch s {
	case "identical":
		return "synced"
	case "ahead", "behind", "diverged":
		return s
	default:
		return s
	}
}

func convertJobs(jobs []ghJob) []JobView {
	out := make([]JobView, 0, len(jobs))
	for _, j := range jobs {
		jv := JobView{
			Name:       j.Name,
			Status:     j.Status,
			Conclusion: j.Conclusion,
			HTMLURL:    j.HTMLURL,
			Steps:      []StepView{},
		}
		for _, s := range j.Steps {
			jv.Steps = append(jv.Steps, StepView{
				Number:     s.Number,
				Name:       s.Name,
				Status:     s.Status,
				Conclusion: s.Conclusion,
			})
		}
		out = append(out, jv)
	}
	return out
}
