package main

import (
	"context"
	"log"
	"sync"
	"time"
)

// SnapshotStore 线程安全的最新快照容器
type SnapshotStore struct {
	mu   sync.RWMutex
	snap Snapshot
}

func NewSnapshotStore() *SnapshotStore { return &SnapshotStore{} }

func (s *SnapshotStore) Set(snap Snapshot) {
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
}

func (s *SnapshotStore) Get() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// Poller 周期轮询 GitHub 并维护内存快照
type Poller struct {
	cfg        Config
	gh         *GitHub
	store      *SnapshotStore
	lastGood   map[string]RepoStatus // 上一轮成功的整仓数据（失败时回填并标 stale）
	lastEvents []EventView
}

func NewPoller(cfg Config, gh *GitHub, store *SnapshotStore) *Poller {
	return &Poller{cfg: cfg, gh: gh, store: store, lastGood: map[string]RepoStatus{}}
}

// Run 立即跑一轮，随后按自适应间隔循环，直到 ctx 取消
func (p *Poller) Run(ctx context.Context) {
	for {
		p.pollOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(p.nextInterval()):
		}
	}
}

func (p *Poller) pollOnce(ctx context.Context) {
	roundCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	results := make([]RepoStatus, len(p.cfg.Repos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6) // 全局并发上限，避免瞬时打爆配额
	for i, repo := range p.cfg.Repos {
		wg.Add(1)
		go func(i int, repo string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = p.fetchRepo(roundCtx, repo)
		}(i, repo)
	}
	wg.Wait()

	now := time.Now().UTC()
	failed := 0
	for i := range results {
		st := results[i]
		if st.Error != "" {
			failed++
			if prev, ok := p.lastGood[st.Name]; ok {
				prev.Stale = true
				prev.Error = st.Error
				results[i] = prev
				continue
			}
		} else {
			t := now
			results[i].LastSuccessAt = &t
			p.lastGood[st.Name] = results[i]
		}
	}

	events, err := p.fetchEvents(roundCtx)
	if err != nil {
		log.Printf("events: %v", err)
		events = p.lastEvents
	} else {
		p.lastEvents = events
	}

	interval := p.nextInterval()
	rate := p.gh.RateLimit()
	log.Printf("poll: %d 仓（失败 %d），配额 %d/%d，下轮 %s 后",
		len(results), failed, rate.Remaining, rate.Limit, interval)

	p.store.Set(Snapshot{
		GeneratedAt:     now,
		PollIntervalSec: int(interval / time.Second),
		TokenConfigured: p.cfg.Token != "",
		Degraded:        p.cfg.Token == "",
		DegradedReason:  degradedReason(p.cfg.Token),
		RateLimit:       RateLimitView(rate),
		Repos:  results,
		Events: events,
	})
}

func degradedReason(token string) string {
	if token == "" {
		return "未配置 GITHUB_TOKEN：匿名配额仅 60 次/小时，已降频至 30 分钟一轮，数据可能滞后"
	}
	return ""
}

// nextInterval 按配额剩余自适应：充裕跟默认间隔、偏紧 60s、紧张 5min、见底退避到重置时刻
func (p *Poller) nextInterval() time.Duration {
	if p.cfg.Token == "" {
		return 30 * time.Minute
	}
	r := p.gh.RateLimit()
	switch {
	case r.Limit == 0:
		return p.cfg.PollInterval
	case r.Remaining > 2000:
		return p.cfg.PollInterval
	case r.Remaining > 500:
		return 60 * time.Second
	case r.Remaining <= 10 && !r.ResetAt.IsZero():
		if d := time.Until(r.ResetAt) + 5*time.Second; d > 0 && d < time.Hour {
			return d
		}
		return 5 * time.Minute
	default:
		return 5 * time.Minute
	}
}
