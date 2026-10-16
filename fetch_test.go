package main

import (
	"testing"
	"time"
)

func revUser(login string) *ghUser { return &ghUser{Login: login} }
func atHour(h int) time.Time       { return time.Date(2026, 1, 1, h, 0, 0, 0, time.UTC) }

func TestPRStatus(t *testing.T) {
	cases := []struct {
		name    string
		reviews []ghReview
		want    string
	}{
		{"无审查为 pending", nil, "pending"},
		{"仅评论不构成决定", []ghReview{
			{User: revUser("a"), State: "COMMENTED", SubmittedAt: atHour(1)},
		}, "pending"},
		{"批准", []ghReview{
			{User: revUser("a"), State: "APPROVED", SubmittedAt: atHour(1)},
		}, "approved"},
		{"后批准覆盖先要改", []ghReview{
			{User: revUser("a"), State: "CHANGES_REQUESTED", SubmittedAt: atHour(1)},
			{User: revUser("a"), State: "APPROVED", SubmittedAt: atHour(2)},
		}, "approved"},
		{"后要改覆盖先批准", []ghReview{
			{User: revUser("a"), State: "APPROVED", SubmittedAt: atHour(1)},
			{User: revUser("a"), State: "CHANGES_REQUESTED", SubmittedAt: atHour(2)},
		}, "changes_requested"},
		{"任一人要改即阻塞", []ghReview{
			{User: revUser("a"), State: "APPROVED", SubmittedAt: atHour(1)},
			{User: revUser("b"), State: "CHANGES_REQUESTED", SubmittedAt: atHour(2)},
		}, "changes_requested"},
		{"无用户信息的 review 忽略", []ghReview{
			{User: nil, State: "APPROVED", SubmittedAt: atHour(1)},
		}, "pending"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PRStatus(tc.reviews); got != tc.want {
				t.Fatalf("PRStatus = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestPickCIRun(t *testing.T) {
	runs := []ghRun{
		{ID: 1, Path: ".github/workflows/release.yml", CreatedAt: atHour(3)},
		{ID: 2, Path: ".github/workflows/ci.yml", CreatedAt: atHour(1)},
		{ID: 3, Path: ".github/workflows/ci.yml", CreatedAt: atHour(2)},
	}
	got := PickCIRun(runs)
	if got == nil || got.ID != 3 {
		t.Fatalf("应跳过 release.yml 并取最新 CI, 得到 %+v", got)
	}
	if PickCIRun(nil) != nil {
		t.Fatal("空列表应返回 nil")
	}
	if PickCIRun([]ghRun{{ID: 9, Path: ".github/workflows/release.yml"}}) != nil {
		t.Fatal("只有 release.yml 时应返回 nil")
	}
}

func TestComputeProgress(t *testing.T) {
	jobs := []ghJob{
		{Name: "build", Status: "in_progress", Steps: []ghStep{
			{Number: 1, Name: "Set up job", Status: "completed", Conclusion: "success"},
			{Number: 2, Name: "Run go test", Status: "in_progress"},
			{Number: 3, Name: "Lint", Status: "queued"},
		}},
	}
	p := ComputeProgress(jobs)
	if p == nil {
		t.Fatal("应有进度")
	}
	if p.TotalSteps != 3 || p.CompletedSteps != 1 || p.Percent != 33 {
		t.Fatalf("进度统计错误: %+v", p)
	}
	if p.CurrentJob != "build" || p.CurrentStep != "Run go test" {
		t.Fatalf("当前 step 定位错误: %+v", p)
	}
	if ComputeProgress(nil) != nil {
		t.Fatal("无 job 应返回 nil")
	}
	if ComputeProgress([]ghJob{{Name: "build", Status: "queued"}}) != nil {
		t.Fatal("job 尚未开始（无 step）应返回 nil")
	}
}

func TestShortSHA(t *testing.T) {
	if got := ShortSHA("a04e6aef1234567890"); got != "a04e6ae" {
		t.Fatalf("ShortSHA = %s", got)
	}
	if got := ShortSHA("abc"); got != "abc" {
		t.Fatalf("短 sha 不应截断: %s", got)
	}
}
