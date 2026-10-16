package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNotFound 目标资源不存在（GitHub 404）
var ErrNotFound = errors.New("not found")

// apiError GitHub 返回的负载错误
type apiError struct {
	Status     int
	Message    string
	RetryAfter time.Duration
}

func (e *apiError) Error() string {
	return fmt.Sprintf("github api %d: %s", e.Status, e.Message)
}

type cachedResp struct {
	ETag string
	Body []byte
}

// RateInfo 配额信息（来自响应头 X-RateLimit-*）
type RateInfo struct {
	Limit     int
	Remaining int
	ResetAt   time.Time
}

// GitHub GitHub REST 客户端。
// 核心机制：ETag 条件请求缓存——第二次起带 If-None-Match，未变化时 GitHub 返回 304，
// 304 不计入 API 配额，这是本项目能以 30s 高频轮询的命脉。
// 注意：GitHub 的 ETag 与 Authorization 头绑定——同一 token 下缓存长期有效，换 token 后首轮全量刷新。
type GitHub struct {
	base  string
	token string
	hc    *http.Client

	mu    sync.Mutex
	cache map[string]*cachedResp
	rate  RateInfo
}

// NewGitHub 创建客户端；token 可为空（匿名模式，配额 60/h，调用方需自行降频）
func NewGitHub(base, token string) *GitHub {
	return &GitHub{
		base:  strings.TrimRight(base, "/"),
		token: token,
		hc:    &http.Client{Timeout: 15 * time.Second},
		cache: make(map[string]*cachedResp),
	}
}

// getJSON 请求 path 并解码到 out，带 ETag 条件请求；404 返回 ErrNotFound
func (g *GitHub) getJSON(ctx context.Context, path string, out any) error {
	url := g.base + path

	g.mu.Lock()
	cached := g.cache[url]
	g.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "tf-console")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	if cached != nil && cached.ETag != "" {
		req.Header.Set("If-None-Match", cached.ETag)
	}

	resp, err := g.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	g.recordRate(resp.Header)

	switch {
	case resp.StatusCode == http.StatusNotModified && cached != nil:
		return json.Unmarshal(cached.Body, out)
	case resp.StatusCode == http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return err
		}
		g.mu.Lock()
		g.cache[url] = &cachedResp{ETag: resp.Header.Get("ETag"), Body: body}
		g.mu.Unlock()
		return json.Unmarshal(body, out)
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		e := &apiError{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				e.RetryAfter = time.Duration(secs) * time.Second
			}
		}
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			if reset := g.RateLimit().ResetAt; !reset.IsZero() {
				if d := time.Until(reset); d > e.RetryAfter {
					e.RetryAfter = d
				}
			}
		}
		return e
	}
}

func (g *GitHub) recordRate(h http.Header) {
	limit, _ := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	remaining, _ := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	reset, _ := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	if limit == 0 {
		return
	}
	g.mu.Lock()
	g.rate = RateInfo{Limit: limit, Remaining: remaining, ResetAt: time.Unix(reset, 0)}
	g.mu.Unlock()
}

// RateLimit 返回最近一次观测到的配额信息
func (g *GitHub) RateLimit() RateInfo {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rate
}
