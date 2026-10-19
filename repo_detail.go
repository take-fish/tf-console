package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	defaultCommitLimit = 30
	maxCommitLimit     = 100
)

// handleRepoBranches GET /api/v1/repos/:repo/branches
func (s *Server) handleRepoBranches(c *gin.Context) {
	repo := c.Param("repo")
	if !s.knownRepo(repo) {
		c.JSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "msg": "unknown repo"})
		return
	}
	if s.gh == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "github client unavailable"})
		return
	}

	branches, err := s.fetchBranches(c.Request.Context(), repo)
	if err != nil {
		writeGitHubAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"branches": branches})
}

// handleRepoCommits GET /api/v1/repos/:repo/commits?branch=master&limit=30
func (s *Server) handleRepoCommits(c *gin.Context) {
	repo := c.Param("repo")
	if !s.knownRepo(repo) {
		c.JSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "msg": "unknown repo"})
		return
	}
	if s.gh == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "github client unavailable"})
		return
	}

	branch := strings.TrimSpace(c.DefaultQuery("branch", "master"))
	if branch == "" || len(branch) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid branch"})
		return
	}
	limit := defaultCommitLimit
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "invalid limit"})
			return
		}
		limit = min(n, maxCommitLimit)
	}

	commits, err := s.fetchBranchCommits(c.Request.Context(), repo, branch, limit)
	if err != nil {
		writeGitHubAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"branch": branch, "commits": commits})
}

func (s *Server) knownRepo(repo string) bool {
	for _, r := range s.cfg.Repos {
		if r == repo {
			return true
		}
	}
	return false
}

func (s *Server) fetchBranches(ctx context.Context, repo string) ([]BranchView, error) {
	var raw []ghBranch
	path := fmt.Sprintf("/repos/%s/%s/branches?per_page=100", s.cfg.Owner, repo)
	if err := s.gh.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}

	branches := make([]BranchView, 0, len(raw))
	for _, b := range raw {
		branches = append(branches, BranchView{
			Name:      b.Name,
			ShortSHA:  ShortSHA(b.Commit.SHA),
			Protected: b.Protected,
		})
	}
	return branches, nil
}

func (s *Server) fetchBranchCommits(
	ctx context.Context,
	repo, branch string,
	limit int,
) ([]CommitView, error) {
	var raw []ghCommit
	path := fmt.Sprintf(
		"/repos/%s/%s/commits?sha=%s&per_page=%d",
		s.cfg.Owner,
		repo,
		url.QueryEscape(branch),
		limit,
	)
	if err := s.gh.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}

	commits := make([]CommitView, 0, len(raw))
	for _, c := range raw {
		if view := toCommitView(c); view != nil {
			commits = append(commits, *view)
		}
	}
	return commits, nil
}

func writeGitHubAPIError(c *gin.Context, err error) {
	var apiErr *apiError
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "msg": "not found"})
	case errors.As(err, &apiErr):
		status := http.StatusBadGateway
		if apiErr.Status == http.StatusForbidden || apiErr.Status == http.StatusTooManyRequests {
			status = http.StatusTooManyRequests
		}
		c.JSON(status, gin.H{"code": status, "msg": apiErr.Message})
	default:
		c.JSON(http.StatusBadGateway, gin.H{"code": http.StatusBadGateway, "msg": err.Error()})
	}
}
