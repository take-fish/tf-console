package main

import (
	"encoding/json"
	"testing"
)

func mkPayload(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestSummarizeEvent(t *testing.T) {
	cases := []struct {
		name    string
		ev      ghEvent
		prTitle string
		want    string
	}{
		{
			"推送",
			ghEvent{Type: "PushEvent", Payload: mkPayload(map[string]any{"size": 2, "ref": "refs/heads/master"})},
			"",
			"推送 2 个提交到 master",
		},
		{
			"打开 PR（标题回填）",
			ghEvent{Type: "PullRequestEvent", Payload: mkPayload(map[string]any{"action": "opened", "number": 3})},
			"feat: 新功能",
			"打开了 PR #3 feat: 新功能",
		},
		{
			"合并 PR（精简 payload 无标题）",
			ghEvent{Type: "PullRequestEvent", Payload: mkPayload(map[string]any{"action": "merged", "number": 1})},
			"",
			"合并了 PR #1",
		},
		{
			"关闭 PR",
			ghEvent{Type: "PullRequestEvent", Payload: mkPayload(map[string]any{"action": "closed", "number": 2})},
			"",
			"关闭了 PR #2",
		},
		{
			"发布",
			ghEvent{Type: "ReleaseEvent", Payload: mkPayload(map[string]any{"release": map[string]any{"tag_name": "v0.1.1"}})},
			"",
			"发布了 v0.1.1",
		},
		{
			"建分支",
			ghEvent{Type: "CreateEvent", Payload: mkPayload(map[string]any{"ref_type": "branch", "ref": "feature/x"})},
			"",
			"创建了分支 feature/x",
		},
		{
			"未知类型原样展示",
			ghEvent{Type: "MysteryEvent"},
			"",
			"MysteryEvent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := summarizeEvent(tc.ev, tc.prTitle); got != tc.want {
				t.Fatalf("summarizeEvent = %q, want %q", got, tc.want)
			}
		})
	}
}
