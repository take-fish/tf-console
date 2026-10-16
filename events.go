package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// fetchEvents 拉取 org 事件流并生成中文摘要。
// 注意：GitHub 官方声明 events API 存在 30s~6h 延迟（非实时），前端需标注。
func (p *Poller) fetchEvents(ctx context.Context) ([]EventView, error) {
	var raw []ghEvent
	path := fmt.Sprintf("/orgs/%s/events?per_page=30", p.cfg.Owner)
	if err := p.gh.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	out := make([]EventView, 0, len(raw))
	seen := map[string]bool{}
	for _, e := range raw {
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		repo := strings.TrimPrefix(e.Repo.Name, p.cfg.Owner+"/")
		if !p.isKnownRepo(repo) {
			continue
		}
		// org 事件流的 pull_request payload 是精简版（无 title），
		// 从已抓取的开放 PR 快照里回填标题（能匹配到才回填，合并后的 PR 匹配不到则只显示编号）
		prTitle := ""
		if e.Type == "PullRequestEvent" {
			var pl struct {
				Number int `json:"number"`
			}
			_ = json.Unmarshal(e.Payload, &pl)
			prTitle = p.findPRTitle(repo, pl.Number)
		}
		out = append(out, EventView{
			ID:          e.ID,
			Type:        e.Type,
			Actor:       e.Actor.Login,
			ActorAvatar: e.Actor.AvatarURL,
			Repo:        repo,
			Summary:     summarizeEvent(e, prTitle),
			CreatedAt:   e.CreatedAt,
		})
		if len(out) >= 50 {
			break
		}
	}
	return out, nil
}

func (p *Poller) isKnownRepo(name string) bool {
	for _, r := range p.cfg.Repos {
		if r == name {
			return true
		}
	}
	return false
}

// findPRTitle 从最近一轮成功快照的开放 PR 中找标题
func (p *Poller) findPRTitle(repo string, number int) string {
	if st, ok := p.lastGood[repo]; ok {
		for _, item := range st.PullRequests.Items {
			if item.Number == number {
				return item.Title
			}
		}
	}
	return ""
}

// summarizeEvent 将事件转换为中文一句话摘要；prTitle 为可选的 PR 标题回填
func summarizeEvent(e ghEvent, prTitle string) string {
	switch e.Type {
	case "PushEvent":
		var p struct {
			Size int    `json:"size"`
			Ref  string `json:"ref"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		return fmt.Sprintf("推送 %d 个提交到 %s", p.Size, strings.TrimPrefix(p.Ref, "refs/heads/"))
	case "PullRequestEvent":
		var p struct {
			Action      string `json:"action"`
			Number      int    `json:"number"`
			PullRequest struct {
				Merged bool `json:"merged"`
			} `json:"pull_request"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		verb := map[string]string{
			"opened": "打开了", "closed": "关闭了", "merged": "合并了", "reopened": "重新打开了",
		}[p.Action]
		if verb == "" {
			verb = p.Action + " 了"
		}
		if p.Action == "closed" && p.PullRequest.Merged {
			verb = "合并了"
		}
		s := fmt.Sprintf("%s PR #%d", verb, p.Number)
		if prTitle != "" {
			s += " " + truncateRunes(prTitle, 40)
		}
		return s
	case "PullRequestReviewEvent":
		var p struct {
			Number int `json:"number"`
			Review struct {
				State string `json:"state"`
			} `json:"review"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		return fmt.Sprintf("审核了 PR #%d（%s）", p.Number, strings.ToLower(p.Review.State))
	case "ReleaseEvent":
		var p struct {
			Release struct {
				TagName string `json:"tag_name"`
			} `json:"release"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		return "发布了 " + p.Release.TagName
	case "CreateEvent":
		var p struct {
			RefType string `json:"ref_type"`
			Ref     string `json:"ref"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		switch p.RefType {
		case "tag":
			return "创建了标签 " + p.Ref
		case "branch":
			return "创建了分支 " + p.Ref
		default:
			return "创建了 " + p.RefType
		}
	case "DeleteEvent":
		var p struct {
			RefType string `json:"ref_type"`
			Ref     string `json:"ref"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		return fmt.Sprintf("删除了 %s %s", p.RefType, p.Ref)
	case "WatchEvent":
		return "标星了仓库"
	case "PublicEvent":
		return "转为公开仓库"
	case "ForkEvent":
		return "Fork 了仓库"
	case "IssueCommentEvent":
		return "评论了 Issue"
	default:
		return e.Type
	}
}

// truncateRunes 按字符截断（对中文安全）
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
